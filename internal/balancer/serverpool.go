package balancer

import (
    "log"
    "net/http"
    "sync/atomic"
    "time"

    "load-balancer/internal/backend"
)

type ServerPool struct {
    backends []*backend.Backend
    current  uint64
}

func NewServerPool() *ServerPool {
    return &ServerPool{}
}

func (serverPool *ServerPool) AddBackend(backend *backend.Backend) {
    serverPool.backends = append(serverPool.backends, backend)
}

func (serverpool *ServerPool) NextIndex() int {
    if len(serverpool.backends) == 0 {
        return 0
    }
    return int(atomic.AddUint64(&serverpool.current, uint64(1)) % uint64(len(serverpool.backends)))
}

func (serverpool *ServerPool) GetNextPeer() *backend.Backend {
    if len(serverpool.backends) == 0 {
        return nil
    }
    
    next := serverpool.NextIndex()
    length := len(serverpool.backends) + next
    for i := next; i < length; i++ {
        idx := i % len(serverpool.backends)
        if serverpool.backends[idx].IsAlive() {
            if i != next {
                atomic.StoreUint64(&serverpool.current, uint64(idx))
            }
            return serverpool.backends[idx]
        }
    }
    return nil
}

func (serverpool *ServerPool) HealthCheck() {
    for _, backend := range serverpool.backends {
        timeout := 2 * time.Second
        client := &http.Client{Timeout: timeout}
        
        alive := false
        resp, err := client.Get(backend.URL.String())
        if err == nil {
            defer resp.Body.Close()
            alive = resp.StatusCode >= 200 && resp.StatusCode < 300
        }

        backend.SetAlive(alive)
        status := "up"
        if !alive {
            status = "down"
        }
        log.Printf("%s [%s]\n", backend.URL, status)
    }
}

func (serverpool *ServerPool) LoadBalancerHandler(writer http.ResponseWriter, request *http.Request) {
    peer := serverpool.GetNextPeer()
    if peer != nil {
        peer.ReverseProxy.ServeHTTP(writer, request)
        return
    }
    http.Error(writer, "Service not available", http.StatusServiceUnavailable)
}
