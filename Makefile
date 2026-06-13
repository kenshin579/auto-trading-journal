.PHONY: run dry backfill-sectors build test vet fmt

# 매매일지 실행 (Go)
run:
	go run ./cmd/atj --log-level INFO

# 드라이런 (시트 미반영)
dry:
	go run ./cmd/atj --dry-run --log-level DEBUG

# 기존 국내 시트 행의 섹터/산업 열 일괄 채움 (1회용)
backfill-sectors:
	go run ./cmd/atj --backfill-sectors --log-level INFO

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
