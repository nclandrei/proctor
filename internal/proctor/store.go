package proctor

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type Store struct {
	Home string
}

func NewStore() (*Store, error) {
	home := os.Getenv("PROCTOR_HOME")
	if home == "" {
		userHome, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		home = filepath.Join(userHome, ".proctor")
	}
	return &Store{Home: home}, nil
}

func (s *Store) RunsRoot() string {
	return filepath.Join(s.Home, "runs")
}

func (s *Store) RepoStateRoot() string {
	return filepath.Join(s.Home, "repos")
}

func (s *Store) RunDir(run Run) string {
	return filepath.Join(s.RunsRoot(), run.RepoSlug, run.ID)
}

func (s *Store) ActiveRunPointer(repoSlug string) string {
	return filepath.Join(s.RepoStateRoot(), repoSlug, "active-run")
}

func (s *Store) SaveRun(run Run) error {
	run.UpdatedAt = time.Now().UTC()
	path := filepath.Join(s.RunDir(run), "run.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func (s *Store) LoadRun(repoRoot string) (Run, error) {
	slug, err := RepoSlug(repoRoot)
	if err != nil {
		return Run{}, err
	}
	ptr, err := os.ReadFile(s.ActiveRunPointer(slug))
	if err != nil {
		return Run{}, fmt.Errorf("load active run: %w", err)
	}
	runPath := strings.TrimSpace(string(ptr))
	data, err := os.ReadFile(filepath.Join(runPath, "run.json"))
	if err != nil {
		return Run{}, err
	}
	var run Run
	if err := json.Unmarshal(data, &run); err != nil {
		return Run{}, err
	}
	if run.Platform == "" {
		run.Platform = run.Surface
	}
	normalizeRunCurlPlan(&run)
	return run, nil
}

func (s *Store) SetActiveRun(run Run) error {
	ptr := s.ActiveRunPointer(run.RepoSlug)
	if err := os.MkdirAll(filepath.Dir(ptr), 0o755); err != nil {
		return err
	}
	return os.WriteFile(ptr, []byte(s.RunDir(run)+"\n"), 0o644)
}

func (s *Store) AppendEvidence(run Run, evidence Evidence) error {
	path := filepath.Join(s.RunDir(run), "evidence.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("lock evidence file: %w", err)
	}
	defer syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
	enc := json.NewEncoder(file)
	return enc.Encode(evidence)
}

func (s *Store) LoadEvidence(run Run) ([]Evidence, error) {
	path := filepath.Join(s.RunDir(run), "evidence.jsonl")
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_SH); err != nil {
		return nil, fmt.Errorf("lock evidence file: %w", err)
	}
	defer syscall.Flock(int(file.Fd()), syscall.LOCK_UN)

	var evidence []Evidence
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var item Evidence
		if err := json.Unmarshal(scanner.Bytes(), &item); err != nil {
			return nil, err
		}
		evidence = append(evidence, item)
	}
	return evidence, scanner.Err()
}

func (s *Store) CopyArtifact(run Run, surface, scenarioID, label, sourcePath string) (Artifact, error) {
	src, err := os.Open(sourcePath)
	if err != nil {
		return Artifact{}, err
	}
	defer src.Close()

	srcInfo, err := src.Stat()
	if err != nil {
		return Artifact{}, err
	}

	ext := filepath.Ext(sourcePath)
	if ext == "" {
		ext = ".dat"
	}
	relativePath, dst, err := s.createArtifactFile(run, surface, scenarioID, label, ext)
	if err != nil {
		return Artifact{}, err
	}
	hasher := sha256.New()
	writer := io.MultiWriter(dst, hasher)
	if _, err := io.Copy(writer, src); err != nil {
		dst.Close()
		os.Remove(dst.Name())
		return Artifact{}, err
	}
	if err := dst.Close(); err != nil {
		os.Remove(dst.Name())
		return Artifact{}, err
	}
	return Artifact{
		Label:       label,
		Path:        relativePath,
		SHA256:      hex.EncodeToString(hasher.Sum(nil)),
		Source:      sourcePath,
		SourceMtime: srcInfo.ModTime().UTC(),
	}, nil
}

func (s *Store) WriteArtifact(run Run, surface, scenarioID, label, extension string, content []byte) (Artifact, error) {
	if extension == "" {
		extension = ".txt"
	}
	relativePath, file, err := s.createArtifactFile(run, surface, scenarioID, label, extension)
	if err != nil {
		return Artifact{}, err
	}
	if _, err := file.Write(content); err != nil {
		file.Close()
		os.Remove(file.Name())
		return Artifact{}, err
	}
	if err := file.Close(); err != nil {
		os.Remove(file.Name())
		return Artifact{}, err
	}
	sum := sha256.Sum256(content)
	return Artifact{
		Label:  label,
		Path:   relativePath,
		SHA256: hex.EncodeToString(sum[:]),
	}, nil
}

func (s *Store) VerifyArtifactHash(run Run, artifact Artifact) error {
	data, err := os.ReadFile(filepath.Join(s.RunDir(run), artifact.Path))
	if err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])
	if got != artifact.SHA256 {
		return fmt.Errorf("hash mismatch for %s", artifact.Path)
	}
	return nil
}

func (s *Store) createArtifactFile(run Run, surface, scenarioID, label, extension string) (string, *os.File, error) {
	dir := filepath.Join(s.RunDir(run), "artifacts", surface, scenarioID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", nil, err
	}
	file, err := os.CreateTemp(dir, fmt.Sprintf("%s-*%s", slugify(label), extension))
	if err != nil {
		return "", nil, err
	}
	relativePath := filepath.Join("artifacts", surface, scenarioID, filepath.Base(file.Name()))
	return relativePath, file, nil
}

func RepoRoot(cwd string) string {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = cwd
	out, err := cmd.Output()
	if err != nil {
		return cwd
	}
	return strings.TrimSpace(string(out))
}

func RepoSlug(repoRoot string) (string, error) {
	cmd := exec.Command("git", "-C", repoRoot, "remote", "get-url", "origin")
	out, err := cmd.Output()
	if err == nil {
		if slug := slugFromRemote(strings.TrimSpace(string(out))); slug != "" {
			return slug, nil
		}
	}
	base := filepath.Base(repoRoot)
	if base == "" || base == "." || base == string(filepath.Separator) {
		return "", fmt.Errorf("derive repo slug")
	}
	return slugify(base), nil
}

func slugFromRemote(remote string) string {
	remote = strings.TrimSuffix(remote, ".git")
	remote = strings.TrimPrefix(remote, "https://github.com/")
	remote = strings.TrimPrefix(remote, "git@github.com:")
	remote = strings.TrimPrefix(remote, "ssh://git@github.com/")
	remote = strings.Trim(remote, "/")
	remote = strings.ReplaceAll(remote, "/", "-")
	return slugify(remote)
}

func slugify(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	result := strings.Trim(b.String(), "-")
	if result == "" {
		return "item"
	}
	return result
}
