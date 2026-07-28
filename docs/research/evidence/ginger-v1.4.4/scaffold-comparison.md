# Comparação dos scaffolds Ginger v1.4.4

Os projetos `serviceprobe --service` e `cliprobe --cli` foram gerados com a
CLI v1.4.4 e `GOTOOLCHAIN=go1.25.0`. Ambos passaram por `go mod tidy`,
`go test ./...`, `go vet ./...` e `go build ./...`.

## Estrutura e entrypoint

| Item | Service | CLI |
| --- | --- | --- |
| entrypoint | `cmd/serviceprobe/main.go` | `cmd/cliprobe/main.go` |
| bootstrap | config → `app.New` → health/routes → `app.Run` | `commands.Execute()` |
| Cobra | não | sim, em `internal/commands` |
| router/health | sim | não |
| teste inicial | `tests/integration/health_test.go` | nenhum |
| Docker/Compose | sim | não |
| Kubernetes/Helm | sim | não |
| GoReleaser | não | `.goreleaser.yaml` |

Hashes SHA-256 dos arquivos comparados:

| Arquivo | SHA-256 |
| --- | --- |
| service Makefile | `79f3dc1ecc4806746886dac758dab64f87c0c47259bd5852eb46476a1c0fb284` |
| CLI Makefile | `e25a3f715a658ca2eeb86850209d30540279c95d766742623feae0131bd765d6` |
| service entrypoint | `7f8b3d00fd6877c05d2d7b78476f1e5b128d465636b02e153f248ba95aa4e016` |
| CLI entrypoint | `0e136d5c8706cd138ca5ba11569c6eaa9cb555c161c805221eea6f11ff11eed0` |
| CLI GoReleaser | `c13b72a64478db24a23f73c821b0dea9f9205544f6af0f7f321e733f33000449` |

## Makefiles

O núcleo dos dois Makefiles é igual: `run`, `build`, `test`, `lint` e `tidy`.
O service acrescenta `docker`, `up` e `down`.

Service:

```make
BIN=bin/serviceprobe

.PHONY: run build test lint tidy docker up down

run:
	go run ./cmd/serviceprobe

build:
	go build -o $(BIN) ./cmd/serviceprobe

test:
	go test ./...

lint:
	golangci-lint run ./...

tidy:
	go mod tidy

docker:
	docker build -f devops/docker/Dockerfile -t serviceprobe:latest .

up:
	docker compose -f devops/docker/docker-compose.yml up -d

down:
	docker compose -f devops/docker/docker-compose.yml down
```

CLI:

```make
BIN=bin/cliprobe

.PHONY: run build test lint tidy

run:
	go run ./cmd/cliprobe

build:
	go build -o $(BIN) ./cmd/cliprobe

test:
	go test ./...

lint:
	golangci-lint run ./...

tidy:
	go mod tidy
```

Nenhum Makefile tem alvo de release. O Kube Peep pode reaproveitar os alvos
comuns, mas seu Makefile deverá integrar frontend, embed, testes e validação do
GoReleaser.

## GoReleaser

Somente o scaffold CLI contém `.goreleaser.yaml`. A configuração usa schema v2,
`CGO_ENABLED=0`, Linux/macOS/Windows, amd64/arm64, `tar.gz` com override ZIP
para Windows, `checksums.txt` e injeta versão, commit e data no pacote Cobra.

O scaffold service não traz GoReleaser. Como o Kube Peep partirá do service,
essa configuração terá de ser incorporada conscientemente na fase de
distribuição, junto do build da SPA e do embed.

## Diagnóstico

- Service: `inspect --json` detectou `/ping`, uma suíte de integração e as
  features Docker/Compose/Helm/Kubernetes; `doctor` saiu com código 0.
- CLI: `inspect --json` não encontrou testes nem rotas; `doctor` saiu com código
  1 somente pela ausência de testes no template.

As saídas completas e sanitizadas estão em `service-inspect.json`,
`service-doctor.txt`, `cli-inspect.json` e `cli-doctor.txt`.
