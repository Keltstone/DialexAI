package engines

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Secrets is the dependency-free built-in engine: no binary, no network.
type Secrets struct {
	SkippedFiles    *[]string
	ApprovedIgnores map[string]int64
}

func (Secrets) Name() string { return "secrets" }

func (s Secrets) Skipped() []string {
	if s.SkippedFiles != nil {
		return *s.SkippedFiles
	}
	return nil
}

func isBinary(data []byte) bool {
	n := len(data)
	if n > 8000 {
		n = 8000
	}
	return bytes.IndexByte(data[:n], 0) != -1
}

var binaryExts = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".ico": true,
	".webp": true, ".pdf": true, ".zip": true, ".tar": true, ".gz": true,
	".7z": true, ".rar": true, ".exe": true, ".dll": true, ".so": true,
	".dylib": true, ".class": true, ".jar": true, ".war": true, ".ear": true,
	".aar": true, ".bin": true, ".iso": true, ".dmg": true, ".mp3": true,
	".mp4": true, ".mov": true, ".avi": true, ".wasm": true, ".pyc": true,
	".o": true, ".a": true, ".keystore": true, ".jks": true,
}

func isBinaryFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return binaryExts[ext]
}

func (s Secrets) Scan(ctx context.Context, root string) ([]Finding, error) {
	fs, sk, err := s.ScanWithSkipped(ctx, root)
	if s.SkippedFiles != nil {
		*s.SkippedFiles = sk
	}
	return fs, err
}

func (s Secrets) ScanWithSkipped(ctx context.Context, root string) ([]Finding, []string, error) {
	var out []Finding
	var skipped []string
	occMap := map[string]int{}

	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		rel, _ := filepath.Rel(root, p)
		if rel == "." {
			return nil
		}
		if d.Name() == ".git" || rel == ".git" || strings.HasPrefix(rel, ".git/") || strings.HasPrefix(rel, ".git\\") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		info, err := os.Lstat(p)
		if err != nil {
			return nil
		}

		// Skip symlinks
		if info.Mode()&os.ModeSymlink != 0 {
			skipped = append(skipped, rel)
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Handle directories
		if d.IsDir() {
			n := d.Name()
			if n == "node_modules" || n == "build" {
				skipped = append(skipped, rel)
				return filepath.SkipDir
			}
			return nil
		}

		// Skip non-regular files
		if !info.Mode().IsRegular() {
			skipped = append(skipped, rel)
			return nil
		}

		// Skip > 1MiB
		if info.Size() > 1<<20 {
			skipped = append(skipped, rel)
			return nil
		}

		f, err := os.Open(p)
		if err != nil {
			return nil
		}
		defer f.Close()

		head := make([]byte, 8000)
		nRead, _ := io.ReadFull(f, head)
		hasNUL := nRead > 0 && isBinary(head[:nRead])
		if hasNUL {
			if !isBinaryFile(rel) {
				// Treat NUL-containing non-binary files as scannable with a warning rather than skipped
				out = append(out, Finding{
					Rule:     "file/nul-byte",
					Engine:   "secrets",
					File:     rel,
					Line:     1,
					Severity: Info,
					Message:  "file contains NUL bytes",
				})
			} else {
				skipped = append(skipped, rel)
				return nil
			}
		}
		// Reset file seek
		f.Seek(0, io.SeekStart)

		// Read lines without bufio.Scanner token limit using bufio.NewReader
		r := bufio.NewReader(f)
		nowUnix := time.Now().Unix()

		for n := 1; ; n++ {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			line, rErr := r.ReadString('\n')
			if len(line) > 0 {
				lineTrimmed := strings.TrimRight(line, "\r\n")

				ruleIgnored := func(ruleID string, fp string) bool {
					if s.ApprovedIgnores == nil || fp == "" {
						return false
					}
					locKey := filepath.ToSlash(rel) + ":" + strconv.Itoa(n)
					ruleKey := locKey + ":" + ruleID + ":" + fp
					if exp, ok := s.ApprovedIgnores[ruleKey]; ok && exp > nowUnix {
						return true
					}
					wildKey := locKey + ":*:" + fp
					if exp, ok := s.ApprovedIgnores[wildKey]; ok && exp > nowUnix {
						return true
					}
					return false
				}

				for _, rRule := range secretRules {
					if m := rRule.Regex.FindString(lineTrimmed); m != "" {
						sev := High
						mHash := sha256.Sum256([]byte(m))
						occKey := fmt.Sprintf("%s|%x", rRule.ID, mHash)
						occ := occMap[occKey]
						fCandidate := Finding{
							Rule:       rRule.ID,
							Engine:     "secrets",
							File:       rel,
							Line:       n,
							Severity:   sev,
							Message:    rRule.Description,
							Evidence:   mask(m),
							Match:      m,
							Occurrence: occ,
						}
						fp := fCandidate.Fingerprint()
						if strings.Contains(lineTrimmed, "sentinix:ignore") && ruleIgnored(rRule.ID, fp) {
							continue
						}
						occMap[occKey]++
						out = append(out, fCandidate)
					}
				}
				// High entropy fallback
				for _, word := range strings.Fields(lineTrimmed) {
					word = strings.Trim(word, `"'=;:,()[]{}<>`)
					if len(word) >= 20 && ShannonEntropy(word) >= 3.6 {
						// Avoid duplicate if already matched by rule
						alreadyMatched := false
						for _, f := range out {
							if f.File == rel && f.Line == n && (strings.Contains(f.Match, word) || strings.Contains(word, f.Match) || strings.Contains(lineTrimmed, f.Match)) {
								alreadyMatched = true
								break
							}
						}
						if !alreadyMatched {
							mHash := sha256.Sum256([]byte(word))
							occKey := fmt.Sprintf("high-entropy|%x", mHash)
							occ := occMap[occKey]
							fCandidate := Finding{
								Rule:       "secret.high-entropy",
								Engine:     "secrets",
								File:       rel,
								Line:       n,
								Severity:   High,
								Message:    "high-entropy secret token",
								Evidence:   mask(word),
								Match:      word,
								Occurrence: occ,
							}
							fp := fCandidate.Fingerprint()
							if strings.Contains(lineTrimmed, "sentinix:ignore") && ruleIgnored("secret.high-entropy", fp) {
								continue
							}
							occMap[occKey]++
							out = append(out, fCandidate)
						}
					}
				}

			}
			if rErr != nil {
				break
			}
		}
		return nil
	})
	return out, skipped, err
}

// placeholder values are documentation and stubs, not credentials. Only the generic rule uses this:
// a real AWS key or private key is never excused by a word in it.
var placeholder = regexp.MustCompile(`(?i)(example|fake|dummy|mock|stub|demo|sample|secure-random|placeholder|changeme|your[-_]|x{6,}|<[^>]+>|\$\{)`)

func isTestPath(p string) bool {
	for _, m := range []string{"/test/", "commonTest", "androidTest", "iosTest", "jvmTest", "_test.go", "Test.kt", "Test.java", "fixtures", "testdata"} {
		if strings.Contains(p, m) {
			return true
		}
	}
	return false
}
