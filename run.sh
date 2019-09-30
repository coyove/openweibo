find cmd/ch -name "*.go" -not -path "*_test.go" | tr '\n' ' ' | xargs go run
