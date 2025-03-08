// Package vfslite
// Copyright (C) 2025 Alex Gaetano Padula & VFSLite Contributors
//
// This library is free software; you can redistribute it and/or
// modify it under the terms of the GNU Lesser General Public
// License as published by the Free Software Foundation; either
// version 2.1 of the License, or (at your option) any later version.
//
// This library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the GNU
// Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public
// License along with this library; if not, write to the Free Software
// Foundation, Inc., 51 Franklin Street, Fifth Floor, Boston, MA  02110-1301  USA
package vfslite

import (
	"bytes"
	"os"
	"testing"
	"time"
)

const (
	testDiskName  = "test_disk.vfs"
	testBlockSize = 4096
)

// cleanup removes the test disk file
func cleanup() {
	_ = os.Remove(testDiskName)
}

// TestVFSOpenClose tests basic open and close operations
func TestVFSOpenClose(t *testing.T) {
	defer cleanup()

	// Open a new VFS
	vfs, err := Open(testDiskName, testBlockSize, 100)
	if err != nil {
		t.Fatalf("Failed to open VFS: %v", err)
	}

	// Close the VFS
	err = vfs.Close()
	if err != nil {
		t.Fatalf("Failed to close VFS: %v", err)
	}

	// Open the same VFS again to ensure persistence
	vfs, err = Open(testDiskName, testBlockSize, 100)
	if err != nil {
		t.Fatalf("Failed to reopen VFS: %v", err)
	}

	// Check that root block is valid
	rootBlock := vfs.GetRootBlock()
	if rootBlock == 0 {
		t.Fatal("Root block should not be 0 (superblock)")
	}

	err = vfs.Close()
	if err != nil {
		t.Fatalf("Failed to close VFS after reopen: %v", err)
	}
}

// TestDirectoryOperations tests creating directories
func TestDirectoryOperations(t *testing.T) {
	defer cleanup()

	vfs, err := Open(testDiskName, testBlockSize, 100)
	if err != nil {
		t.Fatalf("Failed to open VFS: %v", err)
	}
	defer func(vfs *VFSLite) {
		_ = vfs.Close()
	}(vfs)

	rootBlock := vfs.GetRootBlock()

	// Create a directory
	dirName := "test_dir"
	dirBlock, err := vfs.CreateDirectoryBlock(rootBlock, dirName, nil)
	if err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	// Verify directory exists as child of root
	children, err := vfs.ListChildren(rootBlock)
	if err != nil {
		t.Fatalf("Failed to list children of root: %v", err)
	}

	found := false
	for _, child := range children {
		if child == dirBlock {
			found = true
			break
		}
	}

	if !found {
		t.Fatal("Created directory not found as child of root")
	}

	// Verify directory data contains the name
	data, err := vfs.GetBlockData(dirBlock)
	if err != nil {
		t.Fatalf("Failed to get directory data: %v", err)
	}

	if string(data) != dirName {
		t.Fatalf("Directory name mismatch: expected %s, got %s", dirName, string(data))
	}

	// Verify block type
	blockType, err := vfs.GetBlockType(dirBlock)
	if err != nil {
		t.Fatalf("Failed to get block type: %v", err)
	}

	if blockType != BlockTypeDirectory {
		t.Fatalf("Block type mismatch: expected %d, got %d", BlockTypeDirectory, blockType)
	}
}

// TestFileOperations tests creating and reading files
func TestFileOperations(t *testing.T) {
	defer cleanup()

	vfs, err := Open(testDiskName, testBlockSize, 100)
	if err != nil {
		t.Fatalf("Failed to open VFS: %v", err)
	}

	defer func(vfs *VFSLite) {
		_ = vfs.Close()
	}(vfs)

	rootBlock := vfs.GetRootBlock()

	// Create a file
	fileName := "test_file.txt"
	fileContent := []byte("Hello, VFSLite!")
	fileBlock, err := vfs.WriteFile(rootBlock, fileName, fileContent)
	if err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}

	// Verify file exists as child of root
	children, err := vfs.ListChildren(rootBlock)
	if err != nil {
		t.Fatalf("Failed to list children of root: %v", err)
	}

	found := false
	for _, child := range children {
		if child == fileBlock {
			found = true
			break
		}
	}

	if !found {
		t.Fatal("Created file not found as child of root")
	}

	// Verify file data contains the name
	data, err := vfs.GetBlockData(fileBlock)
	if err != nil {
		t.Fatalf("Failed to get file block data: %v", err)
	}

	if string(data) != fileName {
		t.Fatalf("File name mismatch: expected %s, got %s", fileName, string(data))
	}

	// Verify file content
	content, err := vfs.ReadFile(fileBlock)
	if err != nil {
		t.Fatalf("Failed to read file content: %v", err)
	}

	if !bytes.Equal(content, fileContent) {
		t.Fatalf("File content mismatch: expected %s, got %s", fileContent, content)
	}

	// Verify block type
	blockType, err := vfs.GetBlockType(fileBlock)
	if err != nil {
		t.Fatalf("Failed to get block type: %v", err)
	}

	if blockType != BlockTypeFile {
		t.Fatalf("Block type mismatch: expected %d, got %d", BlockTypeFile, blockType)
	}
}

// TestNestedDirectories tests creating and navigating nested directories
func TestNestedDirectories(t *testing.T) {
	defer cleanup()

	vfs, err := Open(testDiskName, testBlockSize, 100)
	if err != nil {
		t.Fatalf("Failed to open VFS: %v", err)
	}

	defer func(vfs *VFSLite) {
		_ = vfs.Close()
	}(vfs)

	rootBlock := vfs.GetRootBlock()

	// Create parent directory
	parentName := "parent"
	parentBlock, err := vfs.CreateDirectoryBlock(rootBlock, parentName, nil)
	if err != nil {
		t.Fatalf("Failed to create parent directory: %v", err)
	}

	// Create child directory inside parent
	childName := "child"
	childBlock, err := vfs.CreateDirectoryBlock(parentBlock, childName, nil)
	if err != nil {
		t.Fatalf("Failed to create child directory: %v", err)
	}

	// Verify parent contains child
	children, err := vfs.ListChildren(parentBlock)
	if err != nil {
		t.Fatalf("Failed to list children of parent: %v", err)
	}

	found := false
	for _, child := range children {
		if child == childBlock {
			found = true
			break
		}
	}

	if !found {
		t.Fatal("Child directory not found as child of parent")
	}

	// Create a file in the child directory
	fileName := "nested_file.txt"
	fileContent := []byte("Nested file content")
	fileBlock, err := vfs.WriteFile(childBlock, fileName, fileContent)
	if err != nil {
		t.Fatalf("Failed to write nested file: %v", err)
	}

	// Verify file is in child directory
	grandchildren, err := vfs.ListChildren(childBlock)
	if err != nil {
		t.Fatalf("Failed to list children of child directory: %v", err)
	}

	found = false
	for _, grandchild := range grandchildren {
		if grandchild == fileBlock {
			found = true
			break
		}
	}

	if !found {
		t.Fatal("Nested file not found in child directory")
	}

	// Read nested file content
	content, err := vfs.ReadFile(fileBlock)
	if err != nil {
		t.Fatalf("Failed to read nested file content: %v", err)
	}

	if !bytes.Equal(content, fileContent) {
		t.Fatalf("Nested file content mismatch: expected %s, got %s", fileContent, content)
	}
}

// TestMetadata tests setting and retrieving metadata
func TestMetadata(t *testing.T) {
	defer cleanup()

	vfs, err := Open(testDiskName, testBlockSize, 100)
	if err != nil {
		t.Fatalf("Failed to open VFS: %v", err)
	}

	defer func(vfs *VFSLite) {
		_ = vfs.Close()
	}(vfs)

	rootBlock := vfs.GetRootBlock()

	// Create metadata
	now := time.Now().Unix()
	meta := &Metadata{
		CreatedOn:  now,
		ModifiedOn: now,
		Extra: map[string]string{
			"author": "test_user",
			"tag":    "important",
		},
	}

	// Create a file with metadata
	fileName := "metadata_file.txt"
	fileContent := []byte("File with metadata")
	fileBlock, err := vfs.CreateFileBlock(rootBlock, fileName, meta)
	if err != nil {
		t.Fatalf("Failed to create file with metadata: %v", err)
	}

	// Add data to the file
	err = vfs.AppendData(fileBlock, fileContent)
	if err != nil {
		t.Fatalf("Failed to append data to file: %v", err)
	}

	// Retrieve and verify metadata
	retrievedMeta, err := vfs.GetBlockMetadata(fileBlock)
	if err != nil {
		t.Fatalf("Failed to get block metadata: %v", err)
	}

	if retrievedMeta.CreatedOn != meta.CreatedOn {
		t.Fatalf("CreatedOn mismatch: expected %d, got %d", meta.CreatedOn, retrievedMeta.CreatedOn)
	}

	author, exists, err := vfs.GetExtraMetadata(fileBlock, "author")
	if err != nil {
		t.Fatalf("Failed to get extra metadata: %v", err)
	}
	if !exists {
		t.Fatal("Expected 'author' metadata to exist")
	}
	if author != "test_user" {
		t.Fatalf("Author mismatch: expected 'test_user', got '%s'", author)
	}

	// Update metadata
	err = vfs.SetExtraMetadata(fileBlock, "version", "1.0")
	if err != nil {
		t.Fatalf("Failed to set extra metadata: %v", err)
	}

	version, exists, err := vfs.GetExtraMetadata(fileBlock, "version")
	if err != nil {
		t.Fatalf("Failed to get updated extra metadata: %v", err)
	}
	if !exists {
		t.Fatal("Expected 'version' metadata to exist")
	}
	if version != "1.0" {
		t.Fatalf("Version mismatch: expected '1.0', got '%s'", version)
	}

	// Verify file content is still intact
	content, err := vfs.ReadFile(fileBlock)
	if err != nil {
		t.Fatalf("Failed to read file content after metadata update: %v", err)
	}

	if !bytes.Equal(content, fileContent) {
		t.Fatalf("File content mismatch after metadata update: expected %s, got %s", fileContent, content)
	}
}

// TestLargeFileStreaming tests streaming API for large files
func TestLargeFileStreaming(t *testing.T) {
	defer cleanup()

	vfs, err := Open(testDiskName, testBlockSize, 500)
	if err != nil {
		t.Fatalf("Failed to open VFS: %v", err)
	}

	defer func(vfs *VFSLite) {
		_ = vfs.Close()
	}(vfs)

	rootBlock := vfs.GetRootBlock()

	// Create a large file (multiple blocks)
	fileName := "large_file.dat"
	fileSize := testBlockSize * 3 // 3 blocks worth of data

	// Create a writer
	writer, err := vfs.CreateStreamWriter(rootBlock, fileName)
	if err != nil {
		t.Fatalf("Failed to create stream writer: %v", err)
	}

	// Write large data in chunks
	chunkSize := testBlockSize / 2
	for i := 0; i < fileSize; i += chunkSize {
		// Generate chunk with ascending values
		chunk := make([]byte, chunkSize)
		for j := 0; j < chunkSize; j++ {
			chunk[j] = byte((i + j) % 256)
		}

		n, err := writer.Write(chunk)
		if err != nil {
			t.Fatalf("Failed to write chunk at offset %d: %v", i, err)
		}
		if n != chunkSize {
			t.Fatalf("Write size mismatch: expected %d, got %d", chunkSize, n)
		}
	}

	// Close the writer
	err = writer.Close()
	if err != nil {
		t.Fatalf("Failed to close writer: %v", err)
	}

	// Get file size
	fileBlock := writer.fileBlock
	size, err := vfs.GetFileSize(fileBlock)
	if err != nil {
		t.Fatalf("Failed to get file size: %v", err)
	}
	if size != uint64(fileSize) {
		t.Fatalf("File size mismatch: expected %d, got %d", fileSize, size)
	}

	// Read the file using streaming API
	reader, err := vfs.OpenStreamReader(fileBlock)
	if err != nil {
		t.Fatalf("Failed to open stream reader: %v", err)
	}

	// Read and verify in chunks
	buffer := make([]byte, chunkSize)
	totalRead := 0
	for totalRead < fileSize {
		n, err := reader.Read(buffer)
		if err != nil {
			t.Fatalf("Failed to read chunk at offset %d: %v", totalRead, err)
		}
		if n <= 0 {
			break
		}

		// Verify chunk content
		for j := 0; j < n; j++ {
			expected := byte((totalRead + j) % 256)
			if buffer[j] != expected {
				t.Fatalf("Data mismatch at offset %d: expected %d, got %d", totalRead+j, expected, buffer[j])
			}
		}

		totalRead += n
	}

	err = reader.Close()
	if err != nil {
		t.Fatalf("Failed to close reader: %v", err)
	}

	if totalRead != fileSize {
		t.Fatalf("Total read size mismatch: expected %d, got %d", fileSize, totalRead)
	}
}

// TestErrorConditions tests various error conditions
func TestErrorConditions(t *testing.T) {
	defer cleanup()

	vfs, err := Open(testDiskName, testBlockSize, 100)
	if err != nil {
		t.Fatalf("Failed to open VFS: %v", err)
	}

	defer func(vfs *VFSLite) {
		_ = vfs.Close()
	}(vfs)

	// Test using superblock as parent (should fail)
	_, err = vfs.CreateDirectoryBlock(0, "invalid", nil)
	if err == nil {
		t.Fatal("Expected error when using superblock as parent, but got none")
	}

	// Test using non-existent block
	_, err = vfs.ListChildren(9999999)
	if err == nil {
		t.Fatal("Expected error when using non-existent block, but got none")
	}

	// Test getting block type for superblock
	_, err = vfs.GetBlockType(0)
	if err == nil {
		t.Fatal("Expected error when getting type of superblock, but got none")
	}

	// Test adding child to superblock
	rootBlock := vfs.GetRootBlock()
	err = vfs.AddChild(0, rootBlock)
	if err == nil {
		t.Fatal("Expected error when adding child to superblock, but got none")
	}

	// Test using invalid block type for streaming
	dirBlock, err := vfs.CreateDirectoryBlock(rootBlock, "test_dir", nil)
	if err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	_, err = vfs.OpenStreamReader(dirBlock)
	if err == nil {
		t.Fatal("Expected error when opening stream reader for non-file block, but got none")
	}
}

// TestMultipleFiles tests creating and managing multiple files
func TestMultipleFiles(t *testing.T) {
	defer cleanup()

	vfs, err := Open(testDiskName, testBlockSize, 100)
	if err != nil {
		t.Fatalf("Failed to open VFS: %v", err)
	}

	defer func(vfs *VFSLite) {
		_ = vfs.Close()
	}(vfs)

	rootBlock := vfs.GetRootBlock()

	// Create multiple files
	fileCount := 10
	fileBlocks := make([]uint64, fileCount)
	for i := 0; i < fileCount; i++ {
		fileName := "file_" + string(rune('A'+i)) + ".txt"
		content := []byte("Content for file " + fileName)
		fileBlock, err := vfs.WriteFile(rootBlock, fileName, content)
		if err != nil {
			t.Fatalf("Failed to create file %d: %v", i, err)
		}
		fileBlocks[i] = fileBlock
	}

	// Verify all files exist as children of root
	children, err := vfs.ListChildren(rootBlock)
	if err != nil {
		t.Fatalf("Failed to list children of root: %v", err)
	}

	if len(children) < fileCount {
		t.Fatalf("Not all files found: expected at least %d, got %d", fileCount, len(children))
	}

	// Read and verify content of all files
	for i, fileBlock := range fileBlocks {
		fileName := "file_" + string(rune('A'+i)) + ".txt"
		expectedContent := []byte("Content for file " + fileName)

		data, err := vfs.GetBlockData(fileBlock)
		if err != nil {
			t.Fatalf("Failed to get file %d name: %v", i, err)
		}

		if string(data) != fileName {
			t.Fatalf("File %d name mismatch: expected %s, got %s", i, fileName, string(data))
		}

		content, err := vfs.ReadFile(fileBlock)
		if err != nil {
			t.Fatalf("Failed to read file %d content: %v", i, err)
		}

		if !bytes.Equal(content, expectedContent) {
			t.Fatalf("File %d content mismatch: expected %s, got %s", i, expectedContent, content)
		}
	}
}
