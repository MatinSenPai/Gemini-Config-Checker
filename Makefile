VERSION ?= $(shell cat VERSION)
PKG     := gemini-config-checker/internal/buildinfo
LDFLAGS := -s -w -X $(PKG).Version=$(VERSION)

# os/arch pairs for the portable web build (static, no CGO)
TARGETS := linux/amd64 linux/arm64 linux/arm windows/amd64 windows/arm64 darwin/amd64 darwin/arm64

.PHONY: test web desktop android dist clean

test:
	go vet . ./internal/...
	go test ./internal/... -count=1

web:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/gemini-config-checker .

# Native window via Wails. Build on the target OS (it needs the platform webview).
desktop:
	cd desktop && wails build -s -clean -trimpath -ldflags "-X $(PKG).Version=$(VERSION)"

# Android APK: the arm64 engine goes into jniLibs, Gradle packages it (needs the Android SDK, JDK 17+ and Gradle 8.10).
android:
	mkdir -p android/app/src/main/jniLibs/arm64-v8a
	CGO_ENABLED=0 GOOS=android GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o android/app/src/main/jniLibs/arm64-v8a/libgccserver.so .
	cd android && VERSION_NAME=$(VERSION) gradle assembleRelease --no-daemon

dist:
	@for t in $(TARGETS); do \
		os=$${t%/*}; arch=$${t#*/}; ext=""; [ $$os = windows ] && ext=".exe"; \
		echo "building $$os/$$arch"; \
		GOARM=7 CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags "$(LDFLAGS)" \
			-o dist/gemini-config-checker-web_$(VERSION)_$${os}_$${arch}$$ext . || exit 1; \
	done

clean:
	rm -rf dist desktop/build/bin
