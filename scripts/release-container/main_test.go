package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func compressedFile(t *testing.T, name string, data []byte) []byte {
	t.Helper()
	return compressedFileMode(t, name, data, 0755)
}

func compressedFileMode(t *testing.T, name string, data []byte, mode int64) []byte {
	t.Helper()
	var b bytes.Buffer
	g := gzip.NewWriter(&b)
	w := tar.NewWriter(g)
	if err := w.WriteHeader(&tar.Header{Name: name, Mode: mode, Size: int64(len(data))}); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestRejectCorruptArchiveGzipFooter(t *testing.T) {
	archive := compressedFile(t, "actionlint", []byte("test actionlint"))
	archive[len(archive)-8] ^= 1
	filename := filepath.Join(t.TempDir(), "actionlint.tar.gz")
	if err := os.WriteFile(filename, archive, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := archiveBinary(filename); err == nil {
		t.Fatal("accepted archive with corrupt gzip footer")
	}
}

func fixture(t *testing.T, mutate func(*imageConfig, *statement), extraLayerNames ...string) verifier {
	t.Helper()
	return fixtureMode(t, mutate, 0755, extraLayerNames...)
}

func fixtureMode(t *testing.T, mutate func(*imageConfig, *statement), mode int64, extraLayerNames ...string) verifier {
	t.Helper()
	root := t.TempDir()
	v := verifier{layout: filepath.Join(root, "layout"), dist: filepath.Join(root, "dist"), output: filepath.Join(root, "metadata"), version: "1.8.0", commit: strings.Repeat("a", 40)}
	for _, dir := range []string{filepath.Join(v.layout, "blobs", "sha256"), v.dist} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(name string, b []byte) {
		t.Helper()
		if err := os.WriteFile(name, b, 0644); err != nil {
			t.Fatal(err)
		}
	}
	blob := func(media string, value any) descriptor {
		t.Helper()
		var b []byte
		if data, ok := value.([]byte); ok {
			b = data
		} else {
			var err error
			b, err = json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
		}
		d := descriptor{MediaType: media, Digest: "sha256:" + sha(b), Size: int64(len(b))}
		write(filepath.Join(v.layout, "blobs", "sha256", sha(b)), b)
		return d
	}
	idx := index{SchemaVersion: 2}
	for _, arch := range []string{"amd64", "arm64"} {
		binary := []byte("test actionlint " + arch)
		write(filepath.Join(v.dist, "actionlint_1.8.0_linux_"+arch+".tar.gz"), compressedFile(t, "actionlint", binary))
		cfg := imageConfig{OS: "linux", Architecture: arch}
		cfg.Config.Labels = map[string]string{"org.opencontainers.image.source": imageSource, "org.opencontainers.image.version": v.version, "org.opencontainers.image.revision": v.commit}
		cfg.Config.Entrypoint = []string{"/usr/local/bin/actionlint"}
		if mutate != nil {
			mutate(&cfg, nil)
		}
		imageLayers := []descriptor{blob(layerType, compressedFileMode(t, "usr/local/bin/actionlint", binary, mode))}
		for _, name := range extraLayerNames {
			imageLayers = append(imageLayers, blob(layerType, compressedFile(t, name, nil)))
		}
		img := blob(manifestType, manifest{SchemaVersion: 2, Config: blob("application/vnd.oci.image.config.v1+json", cfg), Layers: imageLayers})
		img.Platform = platform{"linux", arch}
		layers := []descriptor{}
		for _, predicate := range []string{"https://spdx.dev/Document", "https://slsa.dev/provenance/v1"} {
			s := statement{PredicateType: predicate, Predicate: json.RawMessage("{}")}
			s.Subject = append(s.Subject, struct {
				Digest map[string]string `json:"digest"`
			}{map[string]string{"sha256": strings.TrimPrefix(img.Digest, "sha256:")}})
			if mutate != nil {
				mutate(nil, &s)
			}
			layers = append(layers, blob("application/vnd.in-toto+json", s))
		}
		att := blob(manifestType, manifest{SchemaVersion: 2, Config: blob("application/vnd.oci.image.config.v1+json", []byte("{}")), Layers: layers})
		att.Platform = platform{"unknown", "unknown"}
		att.Annotations = map[string]string{"vnd.docker.reference.type": "attestation-manifest", "vnd.docker.reference.digest": img.Digest}
		idx.Manifests = append(idx.Manifests, img, att)
	}
	rootIndex := index{SchemaVersion: 2, Manifests: []descriptor{blob(indexType, idx)}}
	b, err := json.Marshal(rootIndex)
	if err != nil {
		t.Fatal(err)
	}
	write(filepath.Join(v.layout, "index.json"), b)
	write(filepath.Join(v.layout, "oci-layout"), []byte("{\"imageLayoutVersion\":\"1.0.0\"}"))
	return v
}

func TestVerifyOCI(t *testing.T) {
	v := fixture(t, nil)
	if err := v.verify(); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(v.output, "actionlint_1.8.0_container.json"))
	if err != nil {
		t.Fatal(err)
	}
	m, err := decode[releaseManifest](b)
	if err != nil {
		t.Fatal(err)
	}
	if m.Image != imageName || m.Commit != v.commit || len(m.Platforms) != 2 || !digestPattern.MatchString(m.Digest) {
		t.Fatalf("unexpected manifest: %+v", m)
	}
	for _, p := range m.Platforms {
		for _, name := range []string{p.SBOM, p.Provenance} {
			if _, err := os.Stat(filepath.Join(v.output, name)); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestRejectInvalidOCI(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*imageConfig, *statement)
		change func(*testing.T, *verifier)
		want   string
	}{
		{"source", func(c *imageConfig, _ *statement) {
			if c != nil {
				c.Config.Labels["org.opencontainers.image.revision"] = "wrong"
			}
		}, nil, "identity"},
		{"subject", func(_ *imageConfig, s *statement) {
			if s != nil {
				s.Subject[0].Digest["sha256"] = strings.Repeat("0", 64)
			}
		}, nil, "subject"},
		{"missing sbom", func(_ *imageConfig, s *statement) {
			if s != nil && s.PredicateType == "https://spdx.dev/Document" {
				s.PredicateType = "unknown"
			}
		}, nil, "predicate"},
		{"binary", nil, func(t *testing.T, v *verifier) {
			if err := os.WriteFile(filepath.Join(v.dist, "actionlint_1.8.0_linux_amd64.tar.gz"), compressedFile(t, "actionlint", []byte("different")), 0644); err != nil {
				t.Fatal(err)
			}
		}, "differs"},
		{"blob", nil, func(t *testing.T, v *verifier) {
			entries, err := os.ReadDir(filepath.Join(v.layout, "blobs", "sha256"))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(v.layout, "blobs", "sha256", entries[0].Name()), []byte("bad"), 0644); err != nil {
				t.Fatal(err)
			}
		}, "hash mismatch"},
		{"unexpected file", nil, func(t *testing.T, v *verifier) {
			if err := os.WriteFile(filepath.Join(v.layout, "surprise"), nil, 0644); err != nil {
				t.Fatal(err)
			}
		}, "unexpected OCI"},
		{"symlink", nil, func(t *testing.T, v *verifier) {
			if err := os.Symlink("index.json", filepath.Join(v.layout, "link")); err != nil {
				t.Skip(err)
			}
		}, "non-regular"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := fixture(t, tc.mutate)
			if tc.change != nil {
				tc.change(t, &v)
			}
			err := v.verify()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, wanted %q", err, tc.want)
			}
		})
	}
}

func TestRejectActionlintWhiteouts(t *testing.T) {
	for _, name := range []string{
		".wh.usr", "usr/.wh.local", "usr/local/.wh.bin", "usr/local/bin/.wh.actionlint",
		".wh..wh..opq", "usr/.wh..wh..opq", "usr/local/.wh..wh..opq", "usr/local/bin/.wh..wh..opq",
	} {
		t.Run(name, func(t *testing.T) {
			v := fixture(t, nil, name)
			err := v.verify()
			if err == nil || !strings.Contains(err.Error(), "actionlint path was removed") {
				t.Fatalf("got %v, wanted removal rejection", err)
			}
		})
	}
}

func TestAllowUnrelatedWhiteout(t *testing.T) {
	v := fixture(t, nil, "var/cache/.wh..wh..opq")
	if err := v.verify(); err != nil {
		t.Fatal(err)
	}
}

func TestRejectActionlintParentReplacement(t *testing.T) {
	for _, name := range []string{"usr", "usr/local", "usr/local/bin"} {
		t.Run(name, func(t *testing.T) {
			v := fixture(t, nil, name)
			err := v.verify()
			if err == nil || !strings.Contains(err.Error(), "parent is not a directory") {
				t.Fatalf("got %v, wanted parent replacement rejection", err)
			}
		})
	}
}

func TestRejectNonExecutableActionlint(t *testing.T) {
	v := fixtureMode(t, nil, 0644)
	err := v.verify()
	if err == nil || !strings.Contains(err.Error(), "invalid or repeated image binary") {
		t.Fatalf("got %v, wanted non-executable binary rejection", err)
	}
}

func TestRejectTruncatedLayerGzipFooter(t *testing.T) {
	v := fixture(t, nil)
	if err := v.readLayout(); err != nil {
		t.Fatal(err)
	}
	rootData, err := os.ReadFile(filepath.Join(v.layout, "index.json"))
	if err != nil {
		t.Fatal(err)
	}
	root, err := decode[index](rootData)
	if err != nil {
		t.Fatal(err)
	}
	indexData, err := v.blob(root.Manifests[0])
	if err != nil {
		t.Fatal(err)
	}
	idx, err := decode[index](indexData)
	if err != nil {
		t.Fatal(err)
	}
	image := idx.Manifests[0]
	m, err := v.manifest(image)
	if err != nil {
		t.Fatal(err)
	}
	layer := m.Layers[0]
	truncated := v.blobs[layer.Digest][:len(v.blobs[layer.Digest])-1]
	layer.Digest = "sha256:" + sha(truncated)
	layer.Size = int64(len(truncated))
	v.blobs[layer.Digest] = truncated
	m.Layers[0] = layer
	manifestData, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	image.Digest = "sha256:" + sha(manifestData)
	image.Size = int64(len(manifestData))
	v.blobs[image.Digest] = manifestData
	if _, err := v.image(image); err == nil {
		t.Fatal("accepted image layer with truncated gzip footer")
	}
}
