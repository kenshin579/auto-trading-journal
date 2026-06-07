.PHONY: run dry build test vet fmt

# 매매일지 실행 (Go)
run:
	go run ./cmd/atj --log-level INFO

# 드라이런 (시트 미반영)
dry:
	go run ./cmd/atj --dry-run --log-level DEBUG

# 바이너리 빌드
build:
	go build -o atj ./cmd/atj

# 전체 테스트
test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w .
