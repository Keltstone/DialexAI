package hunt

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEntrypointsMultiPlatform(t *testing.T) {
	tmp := t.TempDir()

	// 1. Android Manifest
	manifest := `<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android" package="com.example.app">
    <application>
        <activity android:name=".MainActivity" android:exported="true">
            <intent-filter>
                <action android:name="android.intent.action.VIEW"/>
                <data android:scheme="myapp" android:host="open"/>
            </intent-filter>
        </activity>
        <service android:name=".SyncService" android:exported="true" />
        <receiver android:name=".BootReceiver" android:exported="false">
            <intent-filter>
                <action android:name="android.intent.action.BOOT_COMPLETED"/>
            </intent-filter>
        </receiver>
        <provider android:name="androidx.core.content.FileProvider" android:authorities="com.example.files" android:exported="true" />
    </application>
</manifest>`
	if err := os.WriteFile(filepath.Join(tmp, "AndroidManifest.xml"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}

	// 2. Android Source with JavascriptInterface and PendingIntent
	srcDir := filepath.Join(tmp, "src", "main", "java")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}
	srcContent := `package com.example.app;
class WebActivity {
    @JavascriptInterface
    public void nativeCall(String data) {}
    void send() {
        PendingIntent.getActivity(ctx, 0, intent, PendingIntent.FLAG_MUTABLE);
    }
}`
	if err := os.WriteFile(filepath.Join(srcDir, "WebActivity.java"), []byte(srcContent), 0644); err != nil {
		t.Fatal(err)
	}

	// 3. iOS Info.plist & Swift file
	iosDir := filepath.Join(tmp, "ios")
	if err := os.MkdirAll(iosDir, 0755); err != nil {
		t.Fatal(err)
	}
	infoPlist := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleURLTypes</key>
    <array>
        <dict>
            <key>CFBundleURLSchemes</key>
            <array>
                <string>myiosapp</string>
            </array>
        </dict>
    </array>
</dict>
</plist>`
	if err := os.WriteFile(filepath.Join(iosDir, "Info.plist"), []byte(infoPlist), 0644); err != nil {
		t.Fatal(err)
	}
	swiftContent := `import Foundation
class ServiceDelegate: NSObject, NSXPCListenerDelegate {
    func listener(_ listener: NSXPCListener, shouldAcceptNewConnection conn: NSXPCConnection) -> Bool {
        return true
    }
}`
	if err := os.WriteFile(filepath.Join(iosDir, "Service.swift"), []byte(swiftContent), 0644); err != nil {
		t.Fatal(err)
	}

	// 4. Backend routes (Go, Python, GraphQL, gRPC)
	backendDir := filepath.Join(tmp, "server")
	if err := os.MkdirAll(backendDir, 0755); err != nil {
		t.Fatal(err)
	}
	goSrc := `package server
import "net/http"
func routes() {
    http.HandleFunc("/api/v1/users", handleUsers)
}`
	if err := os.WriteFile(filepath.Join(backendDir, "server.go"), []byte(goSrc), 0644); err != nil {
		t.Fatal(err)
	}
	pySrc := `from flask import Flask
app = Flask(__name__)
@app.route("/api/login", methods=["POST"])
def login(): pass`
	if err := os.WriteFile(filepath.Join(backendDir, "app.py"), []byte(pySrc), 0644); err != nil {
		t.Fatal(err)
	}
	gqlSrc := `package server
// type Query { getUser: User }
`
	if err := os.WriteFile(filepath.Join(backendDir, "schema.go"), []byte(gqlSrc), 0644); err != nil {
		t.Fatal(err)
	}
	protoSrc := `package server
func register(s *grpc.Server) {
    RegisterAuthServiceServer(s, &srv{})
}`
	if err := os.WriteFile(filepath.Join(backendDir, "auth.go"), []byte(protoSrc), 0644); err != nil {
		t.Fatal(err)
	}

	// Test EntrypointsWithConfig with custom patterns
	cfg := &HuntConfig{
		CustomEntrypoints: []CustomEntrypoint{
			{
				Name:     "custom-hook",
				FileGlob: "server.go",
				Regex:    "handleUsers",
				Kind:     "custom-hook",
			},
		},
	}

	eps, err := EntrypointsWithConfig(tmp, cfg)
	if err != nil {
		t.Fatalf("EntrypointsWithConfig failed: %v", err)
	}

	// Verify found entry points across types
	kindsFound := make(map[string]bool)
	namesFound := make(map[string]bool)
	for _, ep := range eps {
		for _, k := range ep.Kinds {
			kindsFound[k] = true
		}
		namesFound[ep.Name] = true
	}

	expectedKinds := []string{
		"webview", "pending-intent",
		"ios-xpc", "route", "grpc", "custom-hook",
	}
	for _, k := range expectedKinds {
		if !kindsFound[k] {
			t.Errorf("expected kind %q to be discovered, found: %v", k, kindsFound)
		}
	}

	// Verify deterministic sorting (first by file, then by name)
	for i := 1; i < len(eps); i++ {
		prev, curr := eps[i-1], eps[i]
		if prev.File > curr.File {
			t.Errorf("unsorted files: %s > %s", prev.File, curr.File)
		} else if prev.File == curr.File && prev.Name > curr.Name {
			t.Errorf("unsorted names in %s: %s > %s", prev.File, prev.Name, curr.Name)
		}
	}
}
