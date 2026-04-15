package main

import (
	"fmt"
	"grpcapi/internals/api/handlers"
	"grpcapi/internals/api/interceptors"
	pb "grpcapi/proto/gen"
	"log"
	"net"
	"os"
	"time"

	"github.com/joho/godotenv"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func main() {

	// mongodb.CreateMongoClient()

	err := godotenv.Load()
	if err != nil {
		log.Fatalln("Error loading .env file:", err)
	}

	r := interceptors.NewRateLimiter(5, time.Minute)
	s := grpc.NewServer(grpc.ChainUnaryInterceptor(r.RateLimitInterceptor, interceptors.ResponseTimeInterceptor,
		interceptors.AuthenticationInterceptor))

	pb.RegisterExecsServiceServer(s, &handlers.Server{})
	pb.RegisterStudentsServiceServer(s, &handlers.Server{})
	pb.RegisterTeachersServiceServer(s, &handlers.Server{})

	reflection.Register(s)

	port := os.Getenv("SERVER_PORT")

	fmt.Println("Server is running on port:", port)
	lis, err := net.Listen("tcp", port)
	if err != nil {
		log.Fatalln("Error listening on specific port:", err)
	}

	err = s.Serve(lis)
	if err != nil {
		log.Fatalln("Failed to serve:", err)
	}

}
