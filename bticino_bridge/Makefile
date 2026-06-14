.PHONY: build build-web build-go deploy clean dev help

# Default target
all: build

# Build completo (web + go)
build: build-web build-go
	@echo "✅ Build completo exitoso"
	@echo "   Web: web/dist/"
	@echo "   Go:  bticino_bridge"

# Build frontend Svelte
build-web:
	@echo "🔨 Building web frontend..."
	@if [ -d "web" ] && [ -f "web/package.json" ]; then \
		cd web && npm install --silent && npm run build --silent; \
		echo "✅ Web frontend built (web/dist/)"; \
	else \
		echo "⚠️  Web directory not found, skipping web build"; \
	fi

# Build backend Go
build-go:
	@echo "🔨 Building Go binary..."
	GOOS=linux GOARCH=arm GOARM=7 go build -o bticino_bridge ./cmd/main.go
	@echo "✅ Go binary built (bticino_bridge)"

# Deploy a dispositivo (build + deploy)
deploy: build
	@echo "🚀 Deploying to device..."
	./scripts/deploy.sh

# Development mode (hot reload)
dev:
	@echo "🔧 Starting development mode..."
	@echo "   - Web dev server: http://localhost:5173"
	@echo "   - Go backend: http://localhost:8082"
	@echo ""
	@echo "Press Ctrl+C to stop"
	@cd web && npm run dev &
	@go run ./cmd/main.go -config configs/config.yaml

# Install dependencies
install:
	@echo "📦 Installing dependencies..."
	@if [ -d "web" ] && [ -f "web/package.json" ]; then \
		cd web && npm install; \
	fi
	go mod download
	@echo "✅ Dependencies installed"

# Clean build artifacts
clean:
	@echo "🧹 Cleaning..."
	rm -rf web/dist
	rm -rf web/node_modules
	rm -f bticino_bridge
	rm -rf web/.vite
	@echo "✅ Cleaned"

# Run tests
test:
	@echo "🧪 Running tests..."
	./scripts/run_all_tests.sh --all

# Show help
help:
	@echo "BTicino Bridge - Makefile Commands"
	@echo ""
	@echo "Usage: make [command]"
	@echo ""
	@echo "Commands:"
	@echo "  build       Build web frontend + Go binary"
	@echo "  build-web   Build only web frontend"
	@echo "  build-go    Build only Go binary"
	@echo "  deploy      Build + deploy to device"
	@echo "  dev         Development mode (hot reload)"
	@echo "  install     Install dependencies"
	@echo "  clean       Clean build artifacts"
	@echo "  test        Run tests"
	@echo "  help        Show this help"
	@echo ""
	@echo "Examples:"
	@echo "  make build      # Build everything"
	@echo "  make deploy     # Build and deploy"
	@echo "  make dev        # Development mode"
	@echo "  make clean      # Clean everything"
