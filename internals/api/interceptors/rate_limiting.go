package interceptors

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

type rateLimiter struct {
	mu        sync.Mutex
	visitors  map[string]int
	limit     int
	resetTime time.Duration
}

func NewRateLimiter(limit int, resetTime time.Duration) *rateLimiter {
	rl := &rateLimiter{
		visitors:  make(map[string]int),
		limit:     limit,
		resetTime: resetTime,
	}
	// start the reset routine
	go rl.resetVisitorCount()
	return rl
}

func (rl *rateLimiter) resetVisitorCount() {
	for {
		time.Sleep(rl.resetTime)
		rl.mu.Lock()
		rl.visitors = make(map[string]int)
		rl.mu.Unlock()
	}
}

func (rl *rateLimiter) RateLimitInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {

	fmt.Println("Rate Limiter Middleware being returned...")
	rl.mu.Lock()
	defer rl.mu.Unlock()

	p, ok := peer.FromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "unable to get client IP")
	}

	visitorIP := p.Addr.String() // Quizás quieras extraer la IP de una manera más sofisticada.
	rl.visitors[visitorIP]++
	// fmt.Printf("Visitor count from %v is %v\n", visitorIP, rl.visitors[visitorIP])
	log.Printf("*********** Visitor count from IP: %s: %d\n", visitorIP, rl.visitors[visitorIP])

	if rl.visitors[visitorIP] > rl.limit {
		return nil, status.Error(codes.ResourceExhausted, "Too many requests")
	}

	fmt.Println("Rate Limiter ends...")
	return handler(ctx, req)
}
