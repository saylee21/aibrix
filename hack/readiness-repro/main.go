/*
Copyright 2026 The Aibrix Team.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// readiness-repro: does a gateway replica that loses its API server connection
// keep reporting ready, and keep a deleted pod in its cache?
//
// Real kube-apiserver + etcd (envtest), real gateway binary built from main.
//
//	A: reaches the API server through a TCP proxy that we cut.
//	B: direct connection (control).
//	C: like A, plus --model-list-discovery-max-age=10s (the existing opt-in check).
package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

type proxy struct {
	ln     net.Listener
	target string
	mu     sync.Mutex
	conns  []net.Conn
	cut    bool
}

func newProxy(target string) *proxy {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	must(err)
	p := &proxy{ln: ln, target: target}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			p.mu.Lock()
			if p.cut {
				p.mu.Unlock()
				c.Close()
				continue
			}
			up, err := net.Dial("tcp", target)
			if err != nil {
				p.mu.Unlock()
				c.Close()
				continue
			}
			p.conns = append(p.conns, c, up)
			p.mu.Unlock()
			go func() { io.Copy(up, c); up.Close() }()
			go func() { io.Copy(c, up); c.Close() }()
		}
	}()
	return p
}

// Cut drops every live connection and refuses new ones (the port stays open,
// so the gateway sees resets rather than a vanished host).
func (p *proxy) Cut() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cut = true
	for _, c := range p.conns {
		c.Close()
	}
	p.conns = nil
}

func (p *proxy) Restore()     { p.mu.Lock(); p.cut = false; p.mu.Unlock() }
func (p *proxy) Addr() string { return p.ln.Addr().String() }

func kubeconfig(cfg *rest.Config, host, path string) {
	c := clientcmdapi.NewConfig()
	c.Clusters["envtest"] = &clientcmdapi.Cluster{Server: host, CertificateAuthorityData: cfg.CAData}
	c.AuthInfos["admin"] = &clientcmdapi.AuthInfo{ClientCertificateData: cfg.CertData, ClientKeyData: cfg.KeyData, Token: cfg.BearerToken}
	c.Contexts["envtest"] = &clientcmdapi.Context{Cluster: "envtest", AuthInfo: "admin"}
	c.CurrentContext = "envtest"
	must(clientcmd.WriteToFile(*c, path))
}

type gw struct {
	name, grpc, http string
	cmd              *exec.Cmd
}

func startGateway(bin, dir, name, kcfg string, grpcPort, httpPort int, extra ...string) *gw {
	g := &gw{name: name, grpc: fmt.Sprintf("127.0.0.1:%d", grpcPort), http: fmt.Sprintf("127.0.0.1:%d", httpPort)}
	args := append([]string{"--kubeconfig", kcfg, "--grpc-bind-address", g.grpc, "--http-bind-address", g.http,
		"--pprof-bind-address", "", "-v", verbosity()}, extra...)
	g.cmd = exec.Command(bin, args...)
	g.cmd.Env = append(os.Environ(), "REDIS_HOST=127.0.0.1", "REDIS_PORT=6390", "POD_NAME=gateway-"+name, "POD_NAMESPACE=default")
	logf, err := os.Create(filepath.Join(dir, "gateway-"+name+".log"))
	must(err)
	g.cmd.Stdout, g.cmd.Stderr = logf, logf
	must(g.cmd.Start())
	return g
}

func verbosity() string {
	if v := os.Getenv("GW_VERBOSITY"); v != "" {
		return v
	}
	return "2"
}

// pollModels records the first time a gateway's /v1/models stops answering 200.
func pollModels(g *gw, since time.Time, done <-chan struct{}) {
	for {
		select {
		case <-done:
			return
		case <-time.After(1 * time.Second):
		}
		if code, _ := get("http://" + g.http + "/v1/models"); code != 200 {
			fmt.Printf("\n%s: /v1/models first returned %d at %s (%.1f s after the cut)\n",
				g.name, code, time.Now().Format("15:04:05"), time.Since(since).Seconds())
			return
		}
	}
}

// cheapCheck is the reachability check the RFC proposes: one LIST pods with
// limit=1 and a short timeout, using the same kubeconfig as the gateway.
func cheapCheck(name, kcfg string) {
	cfg, err := clientcmd.BuildConfigFromFlags("", kcfg)
	must(err)
	cfg.Timeout = 5 * time.Second
	start := time.Now()
	list, err := kubernetes.NewForConfigOrDie(cfg).CoreV1().Pods("").List(context.Background(), metav1.ListOptions{Limit: 1})
	took := time.Since(start).Round(time.Millisecond)
	if err != nil {
		fmt.Printf("- cheap check via %s kubeconfig: FAILED after %s (%v)\n", name, took, err)
		return
	}
	fmt.Printf("- cheap check via %s kubeconfig: ok in %s, %d item(s) returned\n", name, took, len(list.Items))
}

func health(addr, service string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return "error"
	}
	defer conn.Close()
	r, err := healthpb.NewHealthClient(conn).Check(ctx, &healthpb.HealthCheckRequest{Service: service})
	if err != nil {
		return "unreachable"
	}
	return r.Status.String()
}

func get(url string) (int, string) {
	c := http.Client{Timeout: 15 * time.Second}
	r, err := c.Get(url)
	if err != nil {
		return 0, err.Error()
	}
	defer r.Body.Close()
	b, _ := io.ReadAll(r.Body)
	return r.StatusCode, string(b)
}

var podSeries = regexp.MustCompile(`gateway_pod_outstanding_requests\{[^}]*pod="([^"]+)"`)

func snapshot(label string, gws []*gw) {
	fmt.Printf("\n### %s (%s)\n\n", label, time.Now().Format("15:04:05"))
	fmt.Println("| gateway | probe service \"\" | service gateway-plugin | /v1/models | pods in cache (from /metrics) |")
	fmt.Println("|---|---|---|---|---|")
	for _, g := range gws {
		code, body := get("http://" + g.http + "/v1/models")
		models := fmt.Sprintf("%d", code)
		if code == 200 {
			ids := regexp.MustCompile(`"id":"([^"]+)"`).FindAllStringSubmatch(body, -1)
			var names []string
			for _, m := range ids {
				names = append(names, m[1])
			}
			models = fmt.Sprintf("200 %v", names)
		}
		_, metrics := get("http://" + g.http + "/metrics")
		seen := map[string]bool{}
		for _, m := range podSeries.FindAllStringSubmatch(metrics, -1) {
			seen[m[1]] = true
		}
		var pods []string
		for p := range seen {
			pods = append(pods, p)
		}
		sort.Strings(pods)
		fmt.Printf("| %s | %s | %s | %s | %v |\n", g.name, health(g.grpc, ""), health(g.grpc, "gateway-plugin"), models, pods)
	}
}

func main() {
	out := os.Args[1]
	src, _ := os.Getwd()
	crds, _ := filepath.Glob(filepath.Join(src, "config/crd/*/*.aibrix.ai_*.yaml"))
	env := &envtest.Environment{
		BinaryAssetsDirectory: os.Getenv("KUBEBUILDER_ASSETS"),
		CRDInstallOptions:     envtest.CRDInstallOptions{Paths: crds, ErrorIfPathMissing: true},
	}
	cfg, err := env.Start()
	must(err)
	defer env.Stop()
	apiserver := strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(cfg.Host, "https://"), "http://"), "/")
	fmt.Printf("kube-apiserver (envtest) at %s\n", cfg.Host)

	redis := exec.Command("redis-server", "--port", "6390", "--save", "", "--appendonly", "no")
	must(redis.Start())
	defer redis.Process.Kill()

	kc := kubernetes.NewForConfigOrDie(cfg)
	ctx := context.Background()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "llama-pod-1", Namespace: "default",
			Labels: map[string]string{"model.aibrix.ai/name": "llama-8b", "model.aibrix.ai/port": "8000"}},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "vllm", Image: "vllm/vllm-openai"}}},
	}
	created, err := kc.CoreV1().Pods("default").Create(ctx, pod, metav1.CreateOptions{})
	must(err)
	created.Status = corev1.PodStatus{Phase: corev1.PodRunning, PodIP: "10.0.0.7",
		Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}}
	_, err = kc.CoreV1().Pods("default").UpdateStatus(ctx, created, metav1.UpdateOptions{})
	must(err)

	pA, pC := newProxy(apiserver), newProxy(apiserver)
	kubeconfig(cfg, "https://"+pA.Addr(), filepath.Join(out, "kubeconfig-A"))
	kubeconfig(cfg, cfg.Host, filepath.Join(out, "kubeconfig-B"))
	kubeconfig(cfg, "https://"+pC.Addr(), filepath.Join(out, "kubeconfig-C"))

	bin := filepath.Join(out, "..", "gateway")
	gws := []*gw{
		startGateway(bin, out, "A", filepath.Join(out, "kubeconfig-A"), 50061, 8091),
		startGateway(bin, out, "B", filepath.Join(out, "kubeconfig-B"), 50062, 8092),
		startGateway(bin, out, "C", filepath.Join(out, "kubeconfig-C"), 50063, 8093, "--model-list-discovery-max-age", "10s"),
	}
	defer func() {
		for _, g := range gws {
			g.cmd.Process.Kill()
		}
	}()
	deadline := time.Now().Add(90 * time.Second)
	for _, g := range gws {
		for health(g.grpc, "") != "SERVING" {
			if time.Now().After(deadline) {
				fmt.Println("gateway", g.name, "did not become ready; see its log")
				return
			}
			time.Sleep(500 * time.Millisecond)
		}
	}
	time.Sleep(3 * time.Second)
	snapshot("T0: all three gateways started, one model pod exists", gws)
	fmt.Println()
	for _, n := range []string{"A", "B"} {
		cheapCheck(n, filepath.Join(out, "kubeconfig-"+n))
	}

	pA.Cut()
	pC.Cut()
	cutAt := time.Now()
	fmt.Printf("\nCut the API server connection of A and C at %s\n", cutAt.Format("15:04:05"))
	stopPoll := make(chan struct{})
	defer close(stopPoll)
	go pollModels(gws[2], cutAt, stopPoll)
	time.Sleep(2 * time.Second)
	must(kc.CoreV1().Pods("default").Delete(ctx, "llama-pod-1", metav1.DeleteOptions{GracePeriodSeconds: new(int64)}))
	_, err = kc.CoreV1().Pods("default").Get(ctx, "llama-pod-1", metav1.GetOptions{})
	fmt.Printf("Deleted pod llama-pod-1 at %s; API server now says: %v\n", time.Now().Format("15:04:05"), err)

	time.Sleep(30 * time.Second)
	snapshot("T1: 30 s after the delete, A and C still cut off", gws)
	fmt.Println()
	for _, n := range []string{"A", "B"} {
		cheapCheck(n, filepath.Join(out, "kubeconfig-"+n))
	}
	time.Sleep(60 * time.Second)
	snapshot("T2: 90 s after the delete, A and C still cut off", gws)

	pA.Restore()
	pC.Restore()
	fmt.Printf("\nRestored the connection of A and C at %s\n", time.Now().Format("15:04:05"))
	time.Sleep(45 * time.Second)
	snapshot("T3: 45 s after restoring the connection", gws)
	fmt.Println()
	for _, n := range []string{"A", "B"} {
		cheapCheck(n, filepath.Join(out, "kubeconfig-"+n))
	}
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
