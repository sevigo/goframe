package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidatePath(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	t.Run("valid path", func(t *testing.T) {
		validPath := filepath.Join(tmpDir, "test.txt")
		result, err := ValidatePath(validPath, tmpDir)
		assert.NoError(t, err)
		assert.Equal(t, filepath.Join(tmpDir, "test.txt"), result)
	})

	t.Run("absolute path inside working dir", func(t *testing.T) {
		absTmpDir, err := filepath.Abs(tmpDir)
		require.NoError(t, err)

		validPath := filepath.Join(absTmpDir, "subdir", "test.txt")
		result, err := ValidatePath(validPath, absTmpDir)
		assert.NoError(t, err)
		assert.Contains(t, result, "subdir")
	})

	t.Run("traversal attempt with ..", func(t *testing.T) {
		maliciousPath := filepath.Join(tmpDir, "..", "..", "etc", "passwd")
		_, err := ValidatePath(maliciousPath, tmpDir)
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrPathTraversal)
	})

	t.Run("empty path", func(t *testing.T) {
		_, err := ValidatePath("", tmpDir)
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrEmptyPath)
	})

	t.Run("symlink outside working dir", func(t *testing.T) {
		linkPath := filepath.Join(tmpDir, "link")
		err := os.Symlink("/etc/passwd", linkPath)
		if err != nil {
			t.Skip("cannot create symlink")
		}
		defer os.Remove(linkPath)

		_, err = ValidatePath(linkPath, tmpDir)
		assert.NoError(t, err)
	})
}

func TestFilePartWithWorkingDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	testFile := filepath.Join(tmpDir, "test.go")
	testContent := []byte("package main")
	err = os.WriteFile(testFile, testContent, 0644)
	require.NoError(t, err)

	t.Run("file with working dir", func(t *testing.T) {
		part := FileWithWorkingDir(testFile, tmpDir)
		assert.NotNil(t, part)

		fp, ok := part.(*FilePart)
		require.True(t, ok)
		assert.Equal(t, tmpDir, fp.workingDir)
		assert.Equal(t, testFile, fp.path)
	})

	t.Run("get content with working dir", func(t *testing.T) {
		part := &FilePart{
			path:       testFile,
			workingDir: tmpDir,
		}

		content, err := part.GetContent()
		assert.NoError(t, err)
		assert.Equal(t, testContent, content)
	})

	t.Run("traversal blocked", func(t *testing.T) {
		part := &FilePart{
			path:       filepath.Join(tmpDir, "..", "..", "etc", "passwd"),
			workingDir: tmpDir,
		}

		_, err := part.GetContent()
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrPathTraversal)
	})
}
