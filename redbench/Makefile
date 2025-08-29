OS := $(shell uname -s | tr '[:upper:]' '[:lower:]')
ARCH := $(shell uname -m)

.PHONY: bench

bench:
	GOMAXPROCS=1 go test ./... -run=^$$ -bench='.' -benchmem -benchtime=0.5s -count=6 | tee bench.txt
	@echo "Wrote bench.txt"

