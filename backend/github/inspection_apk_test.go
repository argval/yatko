package github

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"strings"
	"testing"
)

// alpineMember creates real gzip members, optionally omitting tar end records.
func alpineMember(t *testing.T, name, body string, segment bool) []byte {
	t.Helper()
	var raw bytes.Buffer
	tw := tar.NewWriter(&raw)
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0644, Size: int64(len(body))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte(body)); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	b := raw.Bytes()
	if segment {
		b = b[:len(b)-1024]
	}
	var out bytes.Buffer
	gz := gzip.NewWriter(&out)
	if _, err := gz.Write(b); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func androidAPK(t *testing.T, name string) []byte {
	t.Helper()
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	w, err := zw.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("manifest")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestAPKContainers(t *testing.T) {
	for _, tc := range []struct{ raw, want string }{{"x86_64", "amd64"}, {"amd64", "amd64"}, {"aarch64", "arm64"}, {"arm64", "arm64"}, {"x86", "386"}, {"386", "386"}, {"armhf", "arm"}, {"armv7", "arm"}, {"riscv64", "riscv64"}, {"noarch", ""}} {
		t.Run(tc.raw, func(t *testing.T) {
			b := append(alpineMember(t, ".SIGN.RSA.test.rsa.pub", "signature", true), alpineMember(t, ".PKGINFO", "# generated\npkgname = app\npkgver = 1.0-r0\narch = "+tc.raw+"\n", true)...)
			b = append(b, alpineMember(t, "usr/bin/app", "payload", false)...)
			server := newFakeDownloads(t)
			asset := server.add("app.apk", b)
			f := server.client().openAsset(context.Background(), asset.BrowserDownloadURL, asset.Size)
			m := ArtifactMetadata{Role: "unknown", Status: "inspected"}
			if err := inspectAPK(f, &m); err != nil {
				t.Fatal(err)
			}
			want := ArtifactTarget{Platform: "linux", Arch: tc.want}
			if len(m.Targets) != 1 || m.Targets[0] != want || m.Role != "installer" {
				t.Fatalf("%+v", m)
			}
			if f.requests > inspectMaxRequests || f.bytes > inspectMaxBytes {
				t.Fatal("exceeded download budget")
			}
		})
	}
	var m ArtifactMetadata
	if err := inspectAPK(installerFile(androidAPK(t, "AndroidManifest.xml")), &m); err != nil {
		t.Fatal(err)
	}
	if len(m.Targets) != 1 || m.Targets[0] != (ArtifactTarget{Platform: "android"}) || m.Role != "installer" {
		t.Fatalf("%+v", m)
	}
	for _, name := range []string{"nested/AndroidManifest.xml", "README.md"} {
		m = ArtifactMetadata{}
		if err := inspectAPK(installerFile(androidAPK(t, name)), &m); err != nil || m.Status != "unsupported" || m.Role != "metadata" || len(m.Targets) != 0 {
			t.Fatalf("%s: %+v %v", name, m, err)
		}
	}
	for _, magic := range []string{"ADB.", "ADBd", "ADBc", "other"} {
		m = ArtifactMetadata{}
		if err := inspectAPK(installerFile([]byte(magic)), &m); err != nil || m.Status != "unsupported" {
			t.Fatalf("%s: %+v %v", magic, m, err)
		}
		if strings.HasPrefix(magic, "ADB") {
			if len(m.Targets) != 1 || m.Targets[0] != (ArtifactTarget{Platform: "linux"}) {
				t.Fatalf("Alpine format lost its OS constraint: %+v", m)
			}
		} else if len(m.Targets) != 0 {
			t.Fatalf("unknown container acquired a target: %+v", m)
		}
	}
}

func TestAPKInspectionOverridesAndroidFilename(t *testing.T) {
	for _, tc := range []struct {
		name           string
		data           []byte
		platform, role string
	}{
		{"app-android-arm64.apk", alpineMember(t, ".PKGINFO", "arch = x86_64\n", true), "linux", "installer"},
		{"notes-android-arm64.apk", androidAPK(t, "README.md"), "", "metadata"},
		{"next-android-arm64.apk", []byte("ADBd"), "linux", "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := newFakeDownloads(t)
			asset := server.add(tc.name, tc.data)
			m, err := server.client().InspectArtifact(context.Background(), asset)
			if err != nil || m.Role != tc.role {
				t.Fatalf("metadata: %+v, %v", m, err)
			}
			if tc.platform != "" {
				if len(m.Targets) != 1 || m.Targets[0].Platform != tc.platform {
					t.Fatalf("filename overrode container evidence: %+v", m)
				}
			} else if len(m.Targets) != 0 {
				t.Fatalf("non-Android ZIP acquired a target: %+v", m)
			}
			wantStatus := "unsupported"
			if tc.role == "installer" {
				wantStatus = "inspected"
			}
			if m.Status != wantStatus {
				t.Fatalf("status = %q, want %q", m.Status, wantStatus)
			}
		})
	}
}

func TestAPKMalformedAndLimits(t *testing.T) {
	for _, info := range []string{"arch = arm64\narch = amd64\n", "arch=amd64\n", "arch = \n", "pkgname = app\n", "arch = arm 64\n"} {
		b := alpineMember(t, ".PKGINFO", info, true)
		if err := inspectAPK(installerFile(b), &ArtifactMetadata{}); err == nil {
			t.Fatalf("accepted %q", info)
		}
	}
	b := alpineMember(t, ".PKGINFO", "arch = x86_64\n", true)
	if err := inspectAPK(installerFile(b[:len(b)-4]), &ArtifactMetadata{}); err == nil {
		t.Fatal("accepted truncated gzip trailer")
	}
	b = androidAPK(t, "AndroidManifest.xml")
	if err := inspectAPK(installerFile(b[:len(b)-8]), &ArtifactMetadata{}); err == nil {
		t.Fatal("accepted truncated ZIP")
	}
	b = alpineMember(t, ".PKGINFO", strings.Repeat("x", inspectHeadBytes+1), true)
	if err := inspectAPK(installerFile(b), &ArtifactMetadata{}); !errors.Is(err, errInspectLimit) {
		t.Fatalf("large control: %v", err)
	}
	b = alpineMember(t, "unrelated", strings.Repeat("x", inspectMaxInflated+1), false)
	if err := inspectAPK(installerFile(b), &ArtifactMetadata{}); !errors.Is(err, errInspectLimit) {
		t.Fatalf("inflate budget: %v", err)
	}
	b = nil
	for i := 0; i < 9; i++ {
		b = append(b, alpineMember(t, ".SIGN.RSA.test", "signature", true)...)
	}
	if err := inspectAPK(installerFile(b), &ArtifactMetadata{}); !errors.Is(err, errInspectLimit) {
		t.Fatalf("member budget: %v", err)
	}
}
