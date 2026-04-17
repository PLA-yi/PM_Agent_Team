.PHONY: help up down migrate dev-server dev-web dev test build clean seed

help:
	@echo "PMHive — make targets"
	@echo "  up            启动 Postgres (docker-compose)"
	@echo "  down          停止 Postgres"
	@echo "  migrate       执行数据库迁移"
	@echo "  dev-server    启动 Go 后端 (:8080)"
	@echo "  dev-web       启动 React 前端 (:5173)"
	@echo "  dev           并发启动 server + web"
	@echo "  test          运行 Go 单元测试"
	@echo "  build         编译 Go 二进制到 ./bin/server"
	@echo "  seed          插入 demo 任务"
	@echo "  clean         清理 Docker 卷与编译产物"

up:
	docker compose up -d
	@echo "等待 Postgres ready..."
	@for i in 1 2 3 4 5 6 7 8 9 10; do \
		docker compose exec -T postgres pg_isready -U pmhive >/dev/null 2>&1 && echo "Postgres ready" && exit 0; \
		sleep 2; \
	done; echo "Postgres 启动超时"; exit 1

down:
	docker compose down

migrate:
	@cat server/migrations/*.sql | docker compose exec -T postgres psql -U pmhive -d pmhive

dev-server:
	cd server && go run ./cmd/server

dev-web:
	cd web && npm run dev

dev:
	@(cd server && go run ./cmd/server) & \
	(cd web && npm run dev) & \
	wait

test:
	cd server && go test ./...

build:
	cd server && go build -o ../bin/server ./cmd/server

seed:
	@curl -s -X POST http://localhost:8080/api/tasks \
		-H "Content-Type: application/json" \
		-d '{"scenario":"competitor_research","input":"国内 AI 笔记类产品"}' | jq .

clean:
	docker compose down -v
	rm -rf bin
