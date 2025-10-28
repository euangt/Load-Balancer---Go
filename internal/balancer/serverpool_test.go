package balancer

import (
    "bytes"
    "io"
    "log"
    "net/http"
    "net/http/httptest"
    "net/http/httputil"
    "net/url"
    "os"
    "strings"
    "sync"
    "testing"

    "load-balancer/internal/backend"
)

func TestNewServerPool(t *testing.T) {
    pool := NewServerPool()
    
    if pool == nil {
        t.Fatal("NewServerPool() returned nil")
    }
    
    if pool.backends != nil {
        t.Error("Expected backends slice to be nil initially")
    }
    
    if pool.current != 0 {
        t.Errorf("Expected current to be 0, got %d", pool.current)
    }
}

func TestServerPool_AddBackend(t *testing.T) {
    pool := NewServerPool()

    testURL, _ := url.Parse("http://example.com:8080")
    testBackend := &backend.Backend{
        URL:          testURL,
        Alive:        true,
        ReverseProxy: httputil.NewSingleHostReverseProxy(testURL),
    }

    pool.AddBackend(testBackend)
    
    if len(pool.backends) != 1 {
        t.Errorf("Expected 1 backend, got %d", len(pool.backends))
    }
    
    if pool.backends[0] != testBackend {
        t.Error("Backend not added correctly")
    }

    testURL2, _ := url.Parse("http://example2.com:8080")
    testBackend2 := &backend.Backend{
        URL:          testURL2,
        Alive:        true,
        ReverseProxy: httputil.NewSingleHostReverseProxy(testURL2),
    }
    
    pool.AddBackend(testBackend2)
    
    if len(pool.backends) != 2 {
        t.Errorf("Expected 2 backends, got %d", len(pool.backends))
    }
}

func TestServerPool_NextIndex(t *testing.T) {
    pool := NewServerPool()

    for i := 0; i < 3; i++ {
        testURL, _ := url.Parse("http://example.com:808" + string(rune('0'+i)))
        testBackend := &backend.Backend{
            URL:          testURL,
            Alive:        true,
            ReverseProxy: httputil.NewSingleHostReverseProxy(testURL),
        }
        pool.AddBackend(testBackend)
    }

    expectedIndices := []int{1, 2, 0, 1, 2, 0}
    
    for i, expected := range expectedIndices {
        actual := pool.NextIndex()
        if actual != expected {
            t.Errorf("NextIndex() call %d: expected %d, got %d", i+1, expected, actual)
        }
    }
}

func TestServerPool_GetNextPeer(t *testing.T) {
    pool := NewServerPool()

    peer := pool.GetNextPeer()
    if peer != nil {
        t.Error("Expected nil peer from empty pool")
    }

    testURLs := []string{
        "http://example1.com:8080",
        "http://example2.com:8080",
        "http://example3.com:8080",
    }
    
    var backends []*backend.Backend
    for _, urlStr := range testURLs {
        testURL, _ := url.Parse(urlStr)
        testBackend := &backend.Backend{
            URL:          testURL,
            Alive:        true,
            ReverseProxy: httputil.NewSingleHostReverseProxy(testURL),
        }
        backends = append(backends, testBackend)
        pool.AddBackend(testBackend)
    }

    for i := 0; i < 6; i++ {
        peer := pool.GetNextPeer()
        if peer == nil {
            t.Fatalf("GetNextPeer() returned nil at iteration %d", i)
        }
        expectedBackend := backends[(i+1)%3]
        if peer != expectedBackend {
            t.Errorf("Expected backend %s, got %s", expectedBackend.URL.String(), peer.URL.String())
        }
    }
}

func TestServerPool_GetNextPeer_WithDeadBackends(t *testing.T) {
    pool := NewServerPool()

    testURLs := []string{
        "http://example1.com:8080",
        "http://example2.com:8080",
        "http://example3.com:8080",
    }
    
    var backends []*backend.Backend
    for i, urlStr := range testURLs {
        testURL, _ := url.Parse(urlStr)
        alive := i != 1
        testBackend := &backend.Backend{
            URL:          testURL,
            Alive:        alive,
            ReverseProxy: httputil.NewSingleHostReverseProxy(testURL),
        }
        backends = append(backends, testBackend)
        pool.AddBackend(testBackend)
    }

    for i := 0; i < 4; i++ {
        peer := pool.GetNextPeer()
        if peer == nil {
            t.Fatalf("GetNextPeer() returned nil at iteration %d", i)
        }
        
        if peer == backends[1] {
            t.Error("GetNextPeer() returned dead backend")
        }
        
        if !peer.IsAlive() {
            t.Error("GetNextPeer() returned backend that is not alive")
        }
    }
}

func TestServerPool_GetNextPeer_AllDead(t *testing.T) {
    pool := NewServerPool()

    for i := 0; i < 3; i++ {
        testURL, _ := url.Parse("http://example.com:808" + string(rune('0'+i)))
        testBackend := &backend.Backend{
            URL:          testURL,
            Alive:        false,
            ReverseProxy: httputil.NewSingleHostReverseProxy(testURL),
        }
        pool.AddBackend(testBackend)
    }
    
    peer := pool.GetNextPeer()
    if peer != nil {
        t.Error("Expected nil peer when all backends are dead")
    }
}

func TestServerPool_HealthCheck(t *testing.T) {
    var buf bytes.Buffer
    log.SetOutput(&buf)
    defer log.SetOutput(os.Stderr)
    
    pool := NewServerPool()

    testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
    }))
    defer testServer.Close()

    healthyURL, _ := url.Parse(testServer.URL)
    unhealthyURL, _ := url.Parse("http://nonexistent-server:9999")
    
    healthyBackend := &backend.Backend{
        URL:          healthyURL,
        Alive:        false,
        ReverseProxy: httputil.NewSingleHostReverseProxy(healthyURL),
    }
    
    unhealthyBackend := &backend.Backend{
        URL:          unhealthyURL,
        Alive:        true,
        ReverseProxy: httputil.NewSingleHostReverseProxy(unhealthyURL),
    }
    
    pool.AddBackend(healthyBackend)
    pool.AddBackend(unhealthyBackend)

    pool.HealthCheck()

    if !healthyBackend.IsAlive() {
        t.Error("Healthy backend should be alive after health check")
    }
    
    if unhealthyBackend.IsAlive() {
        t.Error("Unhealthy backend should be dead after health check")
    }

    logOutput := buf.String()
    if !strings.Contains(logOutput, "[up]") {
        t.Error("Log should contain '[up]' for healthy backend")
    }
    if !strings.Contains(logOutput, "[down]") {
        t.Error("Log should contain '[down]' for unhealthy backend")
    }
}

func TestServerPool_LoadBalancerHandler(t *testing.T) {
    backendServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        w.Write([]byte("Backend response"))
    }))
    defer backendServer.Close()
    
    pool := NewServerPool()

    req := httptest.NewRequest("GET", "/test", nil)
    rr := httptest.NewRecorder()
    
    pool.LoadBalancerHandler(rr, req)
    
    if rr.Code != http.StatusServiceUnavailable {
        t.Errorf("Expected status 503, got %d", rr.Code)
    }

    backendURL, _ := url.Parse(backendServer.URL)
    testBackend := &backend.Backend{
        URL:          backendURL,
        Alive:        true,
        ReverseProxy: httputil.NewSingleHostReverseProxy(backendURL),
    }
    pool.AddBackend(testBackend)

    req = httptest.NewRequest("GET", "/test", nil)
    rr = httptest.NewRecorder()
    
    pool.LoadBalancerHandler(rr, req)
    
    if rr.Code != http.StatusOK {
        t.Errorf("Expected status 200, got %d", rr.Code)
    }
    
    body, _ := io.ReadAll(rr.Body)
    if string(body) != "Backend response" {
        t.Errorf("Expected 'Backend response', got %s", string(body))
    }
}

func TestServerPool_ConcurrentAccess(t *testing.T) {
    pool := NewServerPool()

    for i := 0; i < 5; i++ {
        testURL, _ := url.Parse("http://example.com:808" + string(rune('0'+i)))
        testBackend := &backend.Backend{
            URL:          testURL,
            Alive:        true,
            ReverseProxy: httputil.NewSingleHostReverseProxy(testURL),
        }
        pool.AddBackend(testBackend)
    }
    
    const numGoroutines = 50
    const numOperations = 100
    
    var wg sync.WaitGroup
    wg.Add(numGoroutines)

    for i := 0; i < numGoroutines; i++ {
        go func() {
            defer wg.Done()
            for j := 0; j < numOperations; j++ {
                peer := pool.GetNextPeer()
                if peer == nil {
                    t.Errorf("GetNextPeer() returned nil during concurrent access")
                    return
                }
            }
        }()
    }
    
    wg.Wait()

    expectedCurrent := uint64(numGoroutines * numOperations)
    if pool.current != expectedCurrent {
        t.Logf("Current counter: %d, expected around: %d", pool.current, expectedCurrent)
    }
}

func TestServerPool_LoadBalancerHandler_Integration(t *testing.T) {
    servers := make([]*httptest.Server, 3)
    for i := 0; i < 3; i++ {
        serverID := i
        servers[i] = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            w.WriteHeader(http.StatusOK)
            w.Write([]byte("Server " + string(rune('0'+serverID))))
        }))
    }
    defer func() {
        for _, server := range servers {
            server.Close()
        }
    }()
    
    pool := NewServerPool()

    for _, server := range servers {
        serverURL, _ := url.Parse(server.URL)
        testBackend := &backend.Backend{
            URL:          serverURL,
            Alive:        true,
            ReverseProxy: httputil.NewSingleHostReverseProxy(serverURL),
        }
        pool.AddBackend(testBackend)
    }

    responses := make(map[string]int)
    
    for i := 0; i < 9; i++ {
        req := httptest.NewRequest("GET", "/test", nil)
        rr := httptest.NewRecorder()
        
        pool.LoadBalancerHandler(rr, req)
        
        if rr.Code != http.StatusOK {
            t.Errorf("Request %d: Expected status 200, got %d", i, rr.Code)
            continue
        }
        
        body, _ := io.ReadAll(rr.Body)
        response := string(body)
        responses[response]++
    }

    for i := 0; i < 3; i++ {
        expectedResponse := "Server " + string(rune('0'+i))
        if responses[expectedResponse] != 3 {
            t.Errorf("Server %d was hit %d times, expected 3", i, responses[expectedResponse])
        }
    }
}

func BenchmarkServerPool_GetNextPeer(b *testing.B) {
    pool := NewServerPool()

    for i := 0; i < 10; i++ {
        testURL, _ := url.Parse("http://example.com:808" + string(rune('0'+i)))
        testBackend := &backend.Backend{
            URL:          testURL,
            Alive:        true,
            ReverseProxy: httputil.NewSingleHostReverseProxy(testURL),
        }
        pool.AddBackend(testBackend)
    }
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        pool.GetNextPeer()
    }
}

func BenchmarkServerPool_NextIndex(b *testing.B) {
    pool := NewServerPool()

    for i := 0; i < 10; i++ {
        testURL, _ := url.Parse("http://example.com:808" + string(rune('0'+i)))
        testBackend := &backend.Backend{
            URL:          testURL,
            Alive:        true,
            ReverseProxy: httputil.NewSingleHostReverseProxy(testURL),
        }
        pool.AddBackend(testBackend)
    }
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        pool.NextIndex()
    }
}