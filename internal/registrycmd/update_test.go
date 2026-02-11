package registrycmd_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/donaldgifford/forge/internal/config"
	"github.com/donaldgifford/forge/internal/registrycmd"
)

// setupGitRegistry creates a temp directory with a scaffolded registry, adds a
// blueprint, initialises a git repo and makes an initial commit. Returns the
// registry directory path.
func setupGitRegistry(t *testing.T) string {
	t.Helper()

	dir := scaffoldRegistry(t)

	_, err := registrycmd.RunBlueprint(&registrycmd.BlueprintOpts{
		RegistryDir: dir,
		Category:    "go",
		Name:        "api",
		Description: "Go API blueprint",
		Tags:        []string{"go", "api"},
	})
	require.NoError(t, err)

	initGitRepo(t, dir)

	// Run update once to seed latest_commit in registry.yaml, then commit.
	result, err := registrycmd.RunUpdate(&registrycmd.UpdateOpts{
		RegistryDir: dir,
	})
	require.NoError(t, err)

	if result.Updated > 0 {
		runGit(t, dir, "add", "-A")
		runGit(t, dir, "commit", "-m", "seed commits")
	}

	return dir
}

func initGitRepo(t *testing.T, dir string) {
	t.Helper()

	runGit(t, dir, "init")
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-m", "init")
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.CommandContext(context.Background(), "git", args...)
	cmd.Dir = dir
	// Set minimal git config for commits in test repos.
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test",
		"GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=test",
		"GIT_COMMITTER_EMAIL=test@test.com",
	)

	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v failed: %s", args, out)
}

func TestRunUpdate_AllUpToDate(t *testing.T) {
	t.Parallel()

	dir := setupGitRegistry(t)

	result, err := registrycmd.RunUpdate(&registrycmd.UpdateOpts{
		RegistryDir: dir,
	})
	require.NoError(t, err)
	assert.Equal(t, 0, result.Stale)
	assert.Equal(t, 0, result.Updated)
	require.NotEmpty(t, result.Reports)

	for _, r := range result.Reports {
		assert.Equal(t, registrycmd.StatusUpToDate, r.Status, "blueprint %s should be up-to-date", r.Path)
	}
}

func TestRunUpdate_VersionChanged(t *testing.T) {
	t.Parallel()

	dir := setupGitRegistry(t)

	// Bump the version in blueprint.yaml.
	bpPath := filepath.Join(dir, "go", "api", "blueprint.yaml")
	bpData, err := os.ReadFile(bpPath)
	require.NoError(t, err)

	updated := strings.Replace(string(bpData), `version: "0.1.0"`, `version: "0.2.0"`, 1)
	require.NoError(t, os.WriteFile(bpPath, []byte(updated), 0o644))

	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-m", "bump version")

	result, err := registrycmd.RunUpdate(&registrycmd.UpdateOpts{
		RegistryDir: dir,
	})
	require.NoError(t, err)
	assert.Positive(t, result.Stale)
	assert.Positive(t, result.Updated)

	// Find the go/api report.
	var report *registrycmd.BlueprintReport
	for i := range result.Reports {
		if result.Reports[i].Path == "go/api" {
			report = &result.Reports[i]

			break
		}
	}

	require.NotNil(t, report, "should have a report for go/api")
	assert.Equal(t, registrycmd.StatusBothChanged, report.Status)
	assert.Equal(t, "0.2.0", report.BlueprintVersion)

	// Verify registry.yaml was updated on disk.
	reg, err := config.LoadRegistry(filepath.Join(dir, "registry.yaml"))
	require.NoError(t, err)

	for _, entry := range reg.Blueprints {
		if entry.Path == "go/api" {
			assert.Equal(t, "0.2.0", entry.Version)
			assert.NotEmpty(t, entry.LatestCommit)
		}
	}
}

func TestRunUpdate_FilesChanged(t *testing.T) {
	t.Parallel()

	dir := setupGitRegistry(t)

	// Modify a template file without changing the version.
	tmplPath := filepath.Join(dir, "go", "api", "{{project_name}}", "README.md.tmpl")
	require.NoError(t, os.WriteFile(tmplPath, []byte("# Updated content\n"), 0o644))

	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-m", "update template")

	result, err := registrycmd.RunUpdate(&registrycmd.UpdateOpts{
		RegistryDir: dir,
	})
	require.NoError(t, err)
	assert.Positive(t, result.Stale)
	assert.Positive(t, result.Updated)

	var report *registrycmd.BlueprintReport
	for i := range result.Reports {
		if result.Reports[i].Path == "go/api" {
			report = &result.Reports[i]

			break
		}
	}

	require.NotNil(t, report)
	assert.Equal(t, registrycmd.StatusFilesChanged, report.Status)

	// Verify registry.yaml commit was updated but version unchanged.
	reg, err := config.LoadRegistry(filepath.Join(dir, "registry.yaml"))
	require.NoError(t, err)

	for _, entry := range reg.Blueprints {
		if entry.Path == "go/api" {
			assert.Equal(t, "0.1.0", entry.Version)
			assert.NotEmpty(t, entry.LatestCommit)
		}
	}
}

func TestRunUpdate_BothChanged(t *testing.T) {
	t.Parallel()

	dir := setupGitRegistry(t)

	// Modify both version and template file.
	bpPath := filepath.Join(dir, "go", "api", "blueprint.yaml")
	bpData, err := os.ReadFile(bpPath)
	require.NoError(t, err)

	updated := strings.Replace(string(bpData), `version: "0.1.0"`, `version: "1.0.0"`, 1)
	require.NoError(t, os.WriteFile(bpPath, []byte(updated), 0o644))

	tmplPath := filepath.Join(dir, "go", "api", "{{project_name}}", "README.md.tmpl")
	require.NoError(t, os.WriteFile(tmplPath, []byte("# v1.0 content\n"), 0o644))

	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-m", "major update")

	result, err := registrycmd.RunUpdate(&registrycmd.UpdateOpts{
		RegistryDir: dir,
	})
	require.NoError(t, err)

	var report *registrycmd.BlueprintReport
	for i := range result.Reports {
		if result.Reports[i].Path == "go/api" {
			report = &result.Reports[i]

			break
		}
	}

	require.NotNil(t, report)
	assert.Equal(t, registrycmd.StatusBothChanged, report.Status)
	assert.Equal(t, "1.0.0", report.BlueprintVersion)

	// Verify both fields updated in registry.yaml.
	reg, err := config.LoadRegistry(filepath.Join(dir, "registry.yaml"))
	require.NoError(t, err)

	for _, entry := range reg.Blueprints {
		if entry.Path == "go/api" {
			assert.Equal(t, "1.0.0", entry.Version)
			assert.NotEmpty(t, entry.LatestCommit)
		}
	}
}

func TestRunUpdate_MissingBlueprint(t *testing.T) {
	t.Parallel()

	dir := setupGitRegistry(t)

	// Load registry, add a bogus entry, re-marshal and write.
	regPath := filepath.Join(dir, "registry.yaml")
	reg, err := config.LoadRegistry(regPath)
	require.NoError(t, err)

	reg.Blueprints = append(reg.Blueprints, config.BlueprintEntry{
		Name:        "python/missing",
		Path:        "python/missing",
		Description: "Missing blueprint",
		Version:     "0.1.0",
		Tags:        []string{"python"},
	})

	data, err := yaml.Marshal(reg)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(regPath, data, 0o644))

	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-m", "add bogus entry")

	result, err := registrycmd.RunUpdate(&registrycmd.UpdateOpts{
		RegistryDir: dir,
	})
	require.NoError(t, err)

	var found bool
	for _, r := range result.Reports {
		if r.Path == "python/missing" {
			found = true
			assert.Equal(t, registrycmd.StatusMissing, r.Status)
		}
	}

	assert.True(t, found, "should have a report for the missing blueprint")
}

func TestRunUpdate_CheckMode_Clean(t *testing.T) {
	t.Parallel()

	dir := setupGitRegistry(t)

	result, err := registrycmd.RunUpdate(&registrycmd.UpdateOpts{
		RegistryDir: dir,
		Check:       true,
	})
	require.NoError(t, err)
	assert.Equal(t, 0, result.Stale)
	assert.Equal(t, 0, result.Updated)
}

func TestRunUpdate_CheckMode_Stale(t *testing.T) {
	t.Parallel()

	dir := setupGitRegistry(t)

	// Record registry.yaml content before update.
	regPath := filepath.Join(dir, "registry.yaml")
	beforeData, err := os.ReadFile(regPath)
	require.NoError(t, err)

	// Bump version to make it stale.
	bpPath := filepath.Join(dir, "go", "api", "blueprint.yaml")
	bpData, err := os.ReadFile(bpPath)
	require.NoError(t, err)

	updated := strings.Replace(string(bpData), `version: "0.1.0"`, `version: "0.3.0"`, 1)
	require.NoError(t, os.WriteFile(bpPath, []byte(updated), 0o644))

	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-m", "bump version for check test")

	result, err := registrycmd.RunUpdate(&registrycmd.UpdateOpts{
		RegistryDir: dir,
		Check:       true,
	})
	require.NoError(t, err)
	assert.Positive(t, result.Stale)
	assert.Equal(t, 0, result.Updated, "check mode should not update any entries")

	// Verify registry.yaml was NOT modified.
	afterData, err := os.ReadFile(regPath)
	require.NoError(t, err)
	assert.Equal(t, string(beforeData), string(afterData), "registry.yaml should not be modified in check mode")
}

func TestRunUpdate_NotGitRepo(t *testing.T) {
	t.Parallel()

	dir := scaffoldRegistry(t)

	_, err := registrycmd.RunUpdate(&registrycmd.UpdateOpts{
		RegistryDir: dir,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "requires a git repository")
}

func TestRunUpdate_MissingRegistryYAML(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	_, err := registrycmd.RunUpdate(&registrycmd.UpdateOpts{
		RegistryDir: dir,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "registry.yaml not found")
}
