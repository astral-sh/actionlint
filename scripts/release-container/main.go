// Command release-container verifies a release OCI layout without running it.
package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

const imageName = "ghcr.io/astral-sh/actionlint"
const imageSource = "https://github.com/astral-sh/actionlint"
const indexType = "application/vnd.oci.image.index.v1+json"
const manifestType = "application/vnd.oci.image.manifest.v1+json"
const layerType = "application/vnd.oci.image.layer.v1.tar+gzip"

var digestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
var commitPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)
var versionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$`)

type descriptor struct {
	MediaType   string            `json:"mediaType"`
	Digest      string            `json:"digest"`
	Size        int64             `json:"size"`
	Platform    platform          `json:"platform"`
	Annotations map[string]string `json:"annotations"`
}
type platform struct {
	OS           string `json:"os"`
	Architecture string `json:"architecture"`
}
type index struct {
	SchemaVersion int          `json:"schemaVersion"`
	Manifests     []descriptor `json:"manifests"`
}
type manifest struct {
	SchemaVersion int          `json:"schemaVersion"`
	Config        descriptor   `json:"config"`
	Layers        []descriptor `json:"layers"`
}
type imageConfig struct {
	OS           string `json:"os"`
	Architecture string `json:"architecture"`
	Config       struct {
		Labels     map[string]string `json:"Labels"`
		Entrypoint []string          `json:"Entrypoint"`
	} `json:"config"`
}
type statement struct {
	PredicateType string `json:"predicateType"`
	Subject       []struct {
		Digest map[string]string `json:"digest"`
	} `json:"subject"`
	Predicate json.RawMessage `json:"predicate"`
}
type imagePlatform struct {
	Platform     string `json:"platform"`
	Digest       string `json:"digest"`
	BinarySHA256 string `json:"binary_sha256"`
	SBOM         string `json:"sbom"`
	Provenance   string `json:"provenance"`
}
type releaseManifest struct {
	Image     string          `json:"image"`
	Version   string          `json:"version"`
	Commit    string          `json:"commit"`
	Digest    string          `json:"digest"`
	Platforms []imagePlatform `json:"platforms"`
}
type verifier struct {
	layout, dist, output, version, commit string
	blobs                                 map[string][]byte
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run(args []string) error {
	f := flag.NewFlagSet("release-container", flag.ContinueOnError)
	v := verifier{}
	f.StringVar(&v.layout, "layout", "", "OCI layout directory")
	f.StringVar(&v.dist, "dist", "", "verified release archive directory")
	f.StringVar(&v.output, "output", "", "container metadata directory")
	f.StringVar(&v.version, "version", "", "release version")
	f.StringVar(&v.commit, "commit", "", "full source commit")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 || v.layout == "" || v.dist == "" || v.output == "" || !versionPattern.MatchString(v.version) || !commitPattern.MatchString(v.commit) {
		return errors.New("expected layout, dist, output, version, and full source commit")
	}
	return v.verify()
}
func sha(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func (v *verifier) readLayout() error {
	v.blobs = map[string][]byte{}
	return filepath.WalkDir(v.layout, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(v.layout, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("non-regular OCI file %q", rel)
		}
		if rel == "index.json" || rel == "oci-layout" {
			return nil
		}
		prefix := "blobs/sha256/"
		if !strings.HasPrefix(rel, prefix) || !digestPattern.MatchString("sha256:"+strings.TrimPrefix(rel, prefix)) {
			return fmt.Errorf("unexpected OCI file %q", rel)
		}
		if info.Size() > 256<<20 {
			return fmt.Errorf("oversized OCI blob %q", rel)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		digest := "sha256:" + sha(b)
		if rel != prefix+strings.TrimPrefix(digest, "sha256:") {
			return fmt.Errorf("OCI blob hash mismatch: %s", rel)
		}
		v.blobs[digest] = b
		return nil
	})
}
func (v *verifier) blob(d descriptor) ([]byte, error) {
	if !digestPattern.MatchString(d.Digest) {
		return nil, fmt.Errorf("invalid OCI digest %q", d.Digest)
	}
	b, ok := v.blobs[d.Digest]
	if !ok || int64(len(b)) != d.Size {
		return nil, fmt.Errorf("missing or wrong-sized OCI blob %s", d.Digest)
	}
	return b, nil
}
func decode[T any](b []byte) (T, error) {
	var result T
	err := json.Unmarshal(b, &result)
	return result, err
}
func (v *verifier) manifest(d descriptor) (manifest, error) {
	if d.MediaType != manifestType {
		return manifest{}, fmt.Errorf("unexpected manifest type %q", d.MediaType)
	}
	b, err := v.blob(d)
	if err != nil {
		return manifest{}, err
	}
	m, err := decode[manifest](b)
	if err != nil {
		return m, err
	}
	if m.SchemaVersion != 2 {
		return m, errors.New("unsupported manifest schema")
	}
	return m, nil
}
func archiveBinary(filename string) ([]byte, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	g, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer g.Close()
	t := tar.NewReader(g)
	var binary []byte
	for {
		h, err := t.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if h.Name != "actionlint" {
			continue
		}
		if binary != nil || h.Typeflag != tar.TypeReg || h.Size > 64<<20 {
			return nil, errors.New("invalid archive binary")
		}
		binary, err = io.ReadAll(t)
		if err != nil {
			return nil, err
		}
	}
	if _, err := io.Copy(io.Discard, g); err != nil {
		return nil, err
	}
	if len(binary) == 0 {
		return nil, errors.New("archive contains no actionlint binary")
	}
	return binary, nil
}
func (v *verifier) image(d descriptor) (string, error) {
	m, err := v.manifest(d)
	if err != nil {
		return "", err
	}
	b, err := v.blob(m.Config)
	if err != nil {
		return "", err
	}
	cfg, err := decode[imageConfig](b)
	if err != nil {
		return "", err
	}
	if cfg.OS != "linux" || cfg.Architecture != d.Platform.Architecture || cfg.Config.Labels["org.opencontainers.image.source"] != imageSource || cfg.Config.Labels["org.opencontainers.image.revision"] != v.commit || cfg.Config.Labels["org.opencontainers.image.version"] != v.version || !slices.Equal(cfg.Config.Entrypoint, []string{"/usr/local/bin/actionlint"}) {
		return "", fmt.Errorf("unexpected image identity/config for %s", d.Digest)
	}
	var found []byte
	for _, layer := range m.Layers {
		if layer.MediaType != layerType {
			return "", fmt.Errorf("unexpected layer type %q", layer.MediaType)
		}
		b, err := v.blob(layer)
		if err != nil {
			return "", err
		}
		g, err := gzip.NewReader(bytes.NewReader(b))
		if err != nil {
			return "", err
		}
		t := tar.NewReader(g)
		for {
			h, err := t.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				g.Close()
				return "", err
			}
			name := path.Clean(strings.TrimPrefix(h.Name, "./"))
			if strings.HasPrefix(name, "/") || name == ".." || strings.HasPrefix(name, "../") {
				g.Close()
				return "", fmt.Errorf("unsafe layer path %q", h.Name)
			}
			if slices.Contains([]string{
				".wh.usr", "usr/.wh.local", "usr/local/.wh.bin", "usr/local/bin/.wh.actionlint",
				".wh..wh..opq", "usr/.wh..wh..opq", "usr/local/.wh..wh..opq", "usr/local/bin/.wh..wh..opq",
			}, name) {
				g.Close()
				return "", errors.New("actionlint path was removed")
			}
			if slices.Contains([]string{"usr", "usr/local", "usr/local/bin"}, name) && h.Typeflag != tar.TypeDir {
				g.Close()
				return "", errors.New("actionlint parent is not a directory")
			}
			if name != "usr/local/bin/actionlint" {
				continue
			}
			if found != nil || h.Typeflag != tar.TypeReg || h.Mode&0111 == 0 || h.Size > 64<<20 {
				g.Close()
				return "", errors.New("invalid or repeated image binary")
			}
			found, err = io.ReadAll(t)
			if err != nil {
				g.Close()
				return "", err
			}
		}
		if _, err := io.Copy(io.Discard, g); err != nil {
			g.Close()
			return "", err
		}
		if err := g.Close(); err != nil {
			return "", err
		}
	}
	want, err := archiveBinary(filepath.Join(v.dist, "actionlint_"+v.version+"_linux_"+d.Platform.Architecture+".tar.gz"))
	if err != nil {
		return "", err
	}
	if !bytes.Equal(found, want) {
		return "", fmt.Errorf("image binary differs from release archive for %s", d.Platform.Architecture)
	}
	return sha(found), nil
}
func (v *verifier) attestations(d descriptor, target, arch string) (string, string, error) {
	m, err := v.manifest(d)
	if err != nil {
		return "", "", err
	}
	if _, err = v.blob(m.Config); err != nil {
		return "", "", err
	}
	var sbom, provenance string
	for _, layer := range m.Layers {
		if layer.MediaType != "application/vnd.in-toto+json" {
			return "", "", fmt.Errorf("unexpected attestation layer %q", layer.MediaType)
		}
		b, err := v.blob(layer)
		if err != nil {
			return "", "", err
		}
		s, err := decode[statement](b)
		if err != nil {
			return "", "", err
		}
		bound := false
		for _, subject := range s.Subject {
			if subject.Digest["sha256"] == strings.TrimPrefix(target, "sha256:") {
				bound = true
			}
		}
		if !bound {
			return "", "", errors.New("attestation subject does not match platform image")
		}
		prefix := "actionlint_" + v.version + "_linux_" + arch
		switch s.PredicateType {
		case "https://spdx.dev/Document":
			if sbom != "" {
				return "", "", errors.New("duplicate SBOM")
			}
			sbom = prefix + "_container.spdx.json"
			if err := writeJSON(v.output, sbom, s.Predicate); err != nil {
				return "", "", err
			}
		case "https://slsa.dev/provenance/v0.2", "https://slsa.dev/provenance/v1":
			if provenance != "" {
				return "", "", errors.New("duplicate provenance")
			}
			provenance = prefix + "_container.provenance.json"
			if err := writeJSON(v.output, provenance, json.RawMessage(b)); err != nil {
				return "", "", err
			}
		default:
			return "", "", fmt.Errorf("unexpected predicate %q", s.PredicateType)
		}
	}
	if sbom == "" || provenance == "" {
		return "", "", errors.New("missing platform SBOM or provenance")
	}
	return sbom, provenance, nil
}
func writeJSON(dir, name string, value any) error {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, name), append(b, '\n'), 0644)
}
func (v *verifier) verify() error {
	if err := v.readLayout(); err != nil {
		return err
	}
	layout, err := os.ReadFile(filepath.Join(v.layout, "oci-layout"))
	if err != nil {
		return err
	}
	var lv struct {
		Version string `json:"imageLayoutVersion"`
	}
	if json.Unmarshal(layout, &lv) != nil || lv.Version != "1.0.0" {
		return errors.New("unsupported OCI layout")
	}
	b, err := os.ReadFile(filepath.Join(v.layout, "index.json"))
	if err != nil {
		return err
	}
	root, err := decode[index](b)
	if err != nil {
		return err
	}
	if root.SchemaVersion != 2 || len(root.Manifests) != 1 || root.Manifests[0].MediaType != indexType {
		return errors.New("expected one OCI image index")
	}
	b, err = v.blob(root.Manifests[0])
	if err != nil {
		return err
	}
	idx, err := decode[index](b)
	if err != nil {
		return err
	}
	if idx.SchemaVersion != 2 || len(idx.Manifests) != 4 {
		return errors.New("expected two platform images and two attestation manifests")
	}
	if err := os.MkdirAll(v.output, 0755); err != nil {
		return err
	}
	result := releaseManifest{Image: imageName, Version: v.version, Commit: v.commit, Digest: root.Manifests[0].Digest}
	for _, arch := range []string{"amd64", "arm64"} {
		var image, att descriptor
		for _, d := range idx.Manifests {
			if d.Platform.OS == "linux" && d.Platform.Architecture == arch {
				if image.Digest != "" {
					return errors.New("duplicate platform")
				}
				image = d
			}
		}
		if image.Digest == "" {
			return fmt.Errorf("missing linux/%s", arch)
		}
		for _, d := range idx.Manifests {
			if d.Annotations["vnd.docker.reference.type"] == "attestation-manifest" && d.Annotations["vnd.docker.reference.digest"] == image.Digest {
				if att.Digest != "" {
					return errors.New("duplicate attestation manifest")
				}
				att = d
			}
		}
		if att.Digest == "" {
			return fmt.Errorf("missing attestations for linux/%s", arch)
		}
		binary, err := v.image(image)
		if err != nil {
			return err
		}
		sbom, provenance, err := v.attestations(att, image.Digest, arch)
		if err != nil {
			return err
		}
		result.Platforms = append(result.Platforms, imagePlatform{"linux/" + arch, image.Digest, binary, sbom, provenance})
	}
	if err := writeJSON(v.output, "actionlint_"+v.version+"_container.json", result); err != nil {
		return err
	}
	fmt.Println(result.Digest)
	return nil
}
