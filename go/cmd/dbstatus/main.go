package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/joho/godotenv"
	_ "github.com/uptrace/bun/driver/pgdriver"
)

func main() {
	ctx := context.Background()
	fmt.Println("PostgreSQL Connection Status:")
	fmt.Println("=============================")

	lbIP := fetchLoadBalancerIP(ctx)
	if lbIP != "" {
		fmt.Printf("🌐 LoadBalancer detected - External IP: %s\n", lbIP)
		dsn := fmt.Sprintf("postgres://postgres:postgres@%s:5432/aro_hcp_embeddings?sslmode=disable", lbIP)
		fmt.Printf("📍 Connection: %s\n\n", dsn)
		if err := testConnection(ctx, dsn); err != nil {
			fmt.Printf("❌ Database connection failed via LoadBalancer: %v\n", err)
		} else {
			fmt.Println("✅ Database connection successful via LoadBalancer")
		}
		return
	}

	_ = godotenv.Load("../manifests/config.env")
	host := getenv("POSTGRES_HOST", "localhost")
	port := getenv("POSTGRES_PORT", "5432")
	dbName := getenv("POSTGRES_DB", "aro_hcp_embeddings")
	user := getenv("POSTGRES_USER", "postgres")
	password := getenv("POSTGRES_PASSWORD", "postgres")

	dsn := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", user, password, host, port, dbName)
	fmt.Printf("🏠 Local/Environment connection\n")
	fmt.Printf("📍 Connection: %s\n\n", dsn)
	if err := testConnection(ctx, dsn); err != nil {
		fmt.Printf("❌ Database connection failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ Database connection successful")
}

func fetchLoadBalancerIP(ctx context.Context) string {
	cmd := exec.CommandContext(ctx, "kubectl", "get", "svc", "postgresql", "-o", "jsonpath={.status.loadBalancer.ingress[0].ip}")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func testConnection(ctx context.Context, dsn string) error {
	db, err := sql.Open("pgdriver", dsn)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := db.Close(); cerr != nil {
			log.Printf("warning: closing connection: %v", cerr)
		}
	}()

	timeout, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return db.PingContext(timeout)
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
