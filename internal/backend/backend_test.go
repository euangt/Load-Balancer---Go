package backend

import (
    "net/url"
    "net/http/httputil"
    "sync"
    "testing"
)

func TestBackend_SetAlive(t *testing.T) {
    tests := []struct {
        name         string
        initialAlive bool
        newStatus    bool
    }{
        {
            name:         "set status to true when initially false",
            initialAlive: false,
            newStatus:    true,
        },
        {
            name:         "set status to false when initially true",
            initialAlive: true,
            newStatus:    false,
        },
        {
            name:         "set status to true when initially true",
            initialAlive: true,
            newStatus:    true,
        },
        {
            name:         "set status to false when initially false",
            initialAlive: false,
            newStatus:    false,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            backend := &Backend{
                Alive: tt.initialAlive,
            }

            backend.SetAlive(tt.newStatus)

            if backend.Alive != tt.newStatus {
                t.Errorf("SetAlive() failed: expected Alive to be %v, got %v", tt.newStatus, backend.Alive)
            }
        })
    }
}

func TestBackend_IsAlive(t *testing.T) {
    tests := []struct {
        name     string
        alive    bool
        expected bool
    }{
        {
            name:     "backend is alive",
            alive:    true,
            expected: true,
        },
        {
            name:     "backend is not alive",
            alive:    false,
            expected: false,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            backend := &Backend{
                Alive: tt.alive,
            }

            result := backend.IsAlive()

            if result != tt.expected {
                t.Errorf("IsAlive() = %v, expected %v", result, tt.expected)
            }
        })
    }
}

func TestBackend_ConcurrentAccess(t *testing.T) {
    backend := &Backend{
        Alive: false,
    }

    const numGoroutines = 100
    const numOperations = 1000

    var wg sync.WaitGroup
    wg.Add(numGoroutines * 2)

    for i := 0; i < numGoroutines; i++ {
        go func() {
            defer wg.Done()
            for j := 0; j < numOperations; j++ {
                backend.IsAlive()
            }
        }()
    }

    for i := 0; i < numGoroutines; i++ {
        go func(goroutineID int) {
            defer wg.Done()
            for j := 0; j < numOperations; j++ {
                status := (goroutineID+j)%2 == 0
                backend.SetAlive(status)
            }
        }(i)
    }

    wg.Wait()

    _ = backend.IsAlive()
}

func TestBackend_StructInitialization(t *testing.T) {
    testURL, err := url.Parse("http://example.com:8080")
    if err != nil {
        t.Fatalf("Failed to parse test URL: %v", err)
    }

    reverseProxy := httputil.NewSingleHostReverseProxy(testURL)

    backend := &Backend{
        URL:          testURL,
        Alive:        true,
        ReverseProxy: reverseProxy,
    }

    if backend.URL.String() != "http://example.com:8080" {
        t.Errorf("URL not set correctly: expected %s, got %s", "http://example.com:8080", backend.URL.String())
    }

    if !backend.Alive {
        t.Error("Alive should be true")
    }

    if backend.ReverseProxy == nil {
        t.Error("ReverseProxy should not be nil")
    }

    if !backend.IsAlive() {
        t.Error("IsAlive() should return true for initially alive backend")
    }
}

func TestBackend_SetAliveAndIsAliveIntegration(t *testing.T) {
    backend := &Backend{
        Alive: false,
    }

    if backend.IsAlive() {
        t.Error("Backend should initially be not alive")
    }

    backend.SetAlive(true)
    if !backend.IsAlive() {
        t.Error("Backend should be alive after SetAlive(true)")
    }

    backend.SetAlive(false)
    if backend.IsAlive() {
        t.Error("Backend should not be alive after SetAlive(false)")
    }

    for i := 0; i < 10; i++ {
        status := i%2 == 0
        backend.SetAlive(status)
        if backend.IsAlive() != status {
            t.Errorf("Iteration %d: IsAlive() = %v, expected %v", i, backend.IsAlive(), status)
        }
    }
}

func BenchmarkBackend_SetAlive(b *testing.B) {
    backend := &Backend{
        Alive: false,
    }

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        backend.SetAlive(i%2 == 0)
    }
}

func BenchmarkBackend_IsAlive(b *testing.B) {
    backend := &Backend{
        Alive: true,
    }

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        backend.IsAlive()
    }
}

func BenchmarkBackend_ConcurrentSetAliveAndIsAlive(b *testing.B) {
    backend := &Backend{
        Alive: false,
    }

    b.ResetTimer()
    b.RunParallel(func(pb *testing.PB) {
        i := 0
        for pb.Next() {
            if i%2 == 0 {
                backend.SetAlive(i%4 == 0)
            } else {
                backend.IsAlive()
            }
            i++
        }
    })
}