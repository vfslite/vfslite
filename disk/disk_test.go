// Package disk tests
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
package disk

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
)

func TestOpenNewDisk(t *testing.T) {
	testFileName := "test.vfsl"

	defer func(name string) {
		_ = os.Remove(name)
	}(testFileName)

	disk, err := Open(testFileName, os.O_RDWR|os.O_CREATE, 0777, 4096, 100, 2, 0.75)
	if err != nil {
		t.Fatalf("Failed to open new disk: %v", err)
	}
	defer func(disk *Disk) {
		_ = disk.Close()
	}(disk)

	if disk.header.signature != Signature {
		t.Errorf("Expected signature %x, got %x", Signature, disk.header.signature)
	}
	if disk.header.version != 1 {
		t.Errorf("Expected version 1, got %d", disk.header.version)
	}
	if disk.header.blockSize != 4096 {
		t.Errorf("Expected block size 4096, got %d", disk.header.blockSize)
	}
	if disk.header.totalBlocks != 100 {
		t.Errorf("Expected total blocks 100, got %d", disk.header.totalBlocks)
	}
}

func TestOpenExistingDisk(t *testing.T) {
	testFileName := "test_existing.vfsl"

	// Create a disk file first
	{
		disk, err := Open(testFileName, os.O_RDWR|os.O_CREATE, 0777, 4096, 100, 2, 0.75)
		if err != nil {
			t.Fatalf("Failed to create initial disk: %v", err)
		}
		_ = disk.Close()
	}

	defer func(name string) {
		_ = os.Remove(name)
	}(testFileName)

	// Now open the existing file
	disk, err := Open(testFileName, os.O_RDWR, 0777, 4096, 100, 2, 0.75)
	if err != nil {
		t.Fatalf("Failed to open existing disk: %v", err)
	}
	defer func(disk *Disk) {
		_ = disk.Close()
	}(disk)

	if disk.header.signature != Signature {
		t.Errorf("Expected signature %x, got %x", Signature, disk.header.signature)
	}
	if disk.header.blockSize != 4096 {
		t.Errorf("Expected block size 4096, got %d", disk.header.blockSize)
	}
}

func TestWriteAndReadBlock(t *testing.T) {
	testFileName := "test_write_read.vfsl"
	defer func(name string) {
		_ = os.Remove(name)
	}(testFileName)

	disk, err := Open(testFileName, os.O_RDWR|os.O_CREATE, 0777, 4096, 100, 2, 0.75)
	if err != nil {
		t.Fatalf("Failed to open disk: %v", err)
	}
	defer func(disk *Disk) {
		_ = disk.Close()
	}(disk)

	// Write data to a block
	testData := []byte("Hello, VFSLite!")
	blockNum, err := disk.Write(testData)
	if err != nil {
		t.Fatalf("Failed to write data: %v", err)
	}

	// Read the data back
	readData, err := disk.ReadAt(blockNum)
	if err != nil {
		t.Fatalf("Failed to read data: %v", err)
	}

	// Verify the first part of the data matches what we wrote
	for i := 0; i < len(testData); i++ {
		if readData[i] != testData[i] {
			t.Errorf("Data mismatch at index %d: expected %v, got %v", i, testData[i], readData[i])
		}
	}

	// The rest should be zeros (padding)
	for i := len(testData); i < int(disk.header.blockSize); i++ {
		if readData[i] != 0 {
			t.Errorf("Expected padding zero at index %d, got %v", i, readData[i])
		}
	}
}

func TestWriteAtAndReadBlock(t *testing.T) {
	testFileName := "test_write_at.vfsl"
	defer func(name string) {
		_ = os.Remove(name)
	}(testFileName)

	disk, err := Open(testFileName, os.O_RDWR|os.O_CREATE, 0777, 4096, 100, 2, 0.75)
	if err != nil {
		t.Fatalf("Failed to open disk: %v", err)
	}
	defer func(disk *Disk) {
		_ = disk.Close()
	}(disk)

	// Write data to a specific block
	testData := []byte("Direct write to block!")
	specificBlock := uint64(42)
	err = disk.WriteAt(testData, specificBlock)
	if err != nil {
		t.Fatalf("Failed to write data at block %d: %v", specificBlock, err)
	}

	// Read the data back
	readData, err := disk.ReadAt(specificBlock)
	if err != nil {
		t.Fatalf("Failed to read data from block %d: %v", specificBlock, err)
	}

	// Verify the first part of the data matches what we wrote
	for i := 0; i < len(testData); i++ {
		if readData[i] != testData[i] {
			t.Errorf("Data mismatch at index %d: expected %v, got %v", i, testData[i], readData[i])
		}
	}
}

func TestDeleteBlock(t *testing.T) {
	testFileName := "test_delete.vfsl"
	defer func(name string) {
		_ = os.Remove(name)
	}(testFileName)

	disk, err := Open(testFileName, os.O_RDWR|os.O_CREATE, 0777, 4096, 100, 2, 0.75)
	if err != nil {
		t.Fatalf("Failed to open disk: %v", err)
	}

	defer func(disk *Disk) {
		_ = disk.Close()
	}(disk)

	// Write data to a block
	testData := []byte("Block to be deleted")
	blockNum, err := disk.Write(testData)
	if err != nil {
		t.Fatalf("Failed to write data: %v", err)
	}

	// Delete the block
	err = disk.Delete(blockNum)
	if err != nil {
		t.Fatalf("Failed to delete block %d: %v", blockNum, err)
	}

	// Check that the block is marked as unallocated in the allocation set
	allocSet, err := disk.readAllocationSet()
	if err != nil {
		t.Fatalf("Failed to read allocation set: %v", err)
	}

	if disk.isBlockAllocated(allocSet, blockNum) {
		t.Errorf("Block %d should be unallocated after deletion", blockNum)
	}

	// The block should be available for reuse
	newBlockNum, err := disk.nextBlock()
	if err != nil {
		t.Fatalf("Failed to get next block: %v", err)
	}

	if newBlockNum != blockNum {
		t.Errorf("Expected to reuse deleted block %d, but got %d", blockNum, newBlockNum)
	}
}

func TestIncreaseDiskSize(t *testing.T) {
	testFileName := "test_increase.vfsl"
	defer func(name string) {
		_ = os.Remove(name)
	}(testFileName)

	// Create a small disk initially
	initialBlocks := uint64(10)
	disk, err := Open(testFileName, os.O_RDWR|os.O_CREATE, 0777, 4096, initialBlocks, 2, 0.75)
	if err != nil {
		t.Fatalf("Failed to open disk: %v", err)
	}

	// Fill the disk to trigger automatic expansion
	for i := 0; i < int(initialBlocks); i++ {
		_, err := disk.Write([]byte("Test data for block expansion"))
		if err != nil {
			t.Fatalf("Failed to write data for block %d: %v", i, err)
		}
	}

	// Try to write one more block, which should trigger expansion
	_, err = disk.Write([]byte("This should trigger expansion"))
	if err != nil {
		t.Fatalf("Failed to write data that should trigger expansion: %v", err)
	}

	// Verify disk has expanded
	if disk.header.totalBlocks <= initialBlocks {
		t.Errorf("Disk did not expand as expected. Total blocks: %d, Initial blocks: %d",
			disk.header.totalBlocks, initialBlocks)
	}

	expectedBlocks := initialBlocks * uint64(disk.multiplier)
	if disk.header.totalBlocks != expectedBlocks {
		t.Errorf("Expected %d blocks after expansion, got %d", expectedBlocks, disk.header.totalBlocks)
	}

	_ = disk.Close()
}

func TestBlockToOffset(t *testing.T) {
	testFileName := "test_offset.vfsl"
	defer func(name string) {
		_ = os.Remove(name)
	}(testFileName)

	disk, err := Open(testFileName, os.O_RDWR|os.O_CREATE, 0777, 4096, 100, 2, 0.75)
	if err != nil {
		t.Fatalf("Failed to open disk: %v", err)
	}

	defer func(disk *Disk) {
		_ = disk.Close()
	}(disk)

	// Calculate expected offsets
	expectedOffset0 := int64(disk.header.dataBlocksOffset)
	expectedOffset1 := int64(disk.header.dataBlocksOffset + uint64(disk.header.blockSize))
	expectedOffset99 := int64(disk.header.dataBlocksOffset + 99*uint64(disk.header.blockSize))

	// Check offset calculations
	if offset := disk.blockToOffset(0); offset != expectedOffset0 {
		t.Errorf("Expected offset %d for block 0, got %d", expectedOffset0, offset)
	}
	if offset := disk.blockToOffset(1); offset != expectedOffset1 {
		t.Errorf("Expected offset %d for block 1, got %d", expectedOffset1, offset)
	}
	if offset := disk.blockToOffset(99); offset != expectedOffset99 {
		t.Errorf("Expected offset %d for block 99, got %d", expectedOffset99, offset)
	}
}

func TestIsBlockAllocated(t *testing.T) {
	testFileName := "test_allocation.vfsl"
	defer func(name string) {
		_ = os.Remove(name)
	}(testFileName)

	disk, err := Open(testFileName, os.O_RDWR|os.O_CREATE, 0777, 4096, 100, 2, 0.75)
	if err != nil {
		t.Fatalf("Failed to open disk: %v", err)
	}

	defer func(disk *Disk) {
		_ = disk.Close()
	}(disk)

	// Get initial allocation set
	allocSet, err := disk.readAllocationSet()
	if err != nil {
		t.Fatalf("Failed to read allocation set: %v", err)
	}

	// Initially all blocks should be unallocated
	for i := uint64(0); i < disk.header.totalBlocks; i++ {
		if disk.isBlockAllocated(allocSet, i) {
			t.Errorf("Block %d should be unallocated initially", i)
		}
	}

	// Write to a block to allocate it
	blockNum, err := disk.Write([]byte("Test allocation"))
	if err != nil {
		t.Fatalf("Failed to write data: %v", err)
	}

	// Read updated allocation set
	allocSet, err = disk.readAllocationSet()
	if err != nil {
		t.Fatalf("Failed to read allocation set: %v", err)
	}

	// Check that our block is now allocated
	if !disk.isBlockAllocated(allocSet, blockNum) {
		t.Errorf("Block %d should be allocated after writing", blockNum)
	}
}

func TestMultipleWrites(t *testing.T) {
	testFileName := "test_multiple_writes.vfsl"
	defer func(name string) {
		_ = os.Remove(name)
	}(testFileName)

	disk, err := Open(testFileName, os.O_RDWR|os.O_CREATE, 0777, 4096, 100, 2, 0.75)
	if err != nil {
		t.Fatalf("Failed to open disk: %v", err)
	}

	defer func(disk *Disk) {
		_ = disk.Close()
	}(disk)

	// Write to multiple blocks
	var blocks []uint64
	for i := 0; i < 10; i++ {
		data := []byte("Test data for block " + string(rune('0'+i)))
		blockNum, err := disk.Write(data)
		if err != nil {
			t.Fatalf("Failed to write data for block %d: %v", i, err)
		}
		blocks = append(blocks, blockNum)
	}

	// Read back and verify all blocks
	for i, blockNum := range blocks {
		expectedPrefix := []byte("Test data for block " + string(rune('0'+i)))
		data, err := disk.ReadAt(blockNum)
		if err != nil {
			t.Fatalf("Failed to read data from block %d: %v", blockNum, err)
		}

		// Check prefix matches
		for j := 0; j < len(expectedPrefix); j++ {
			if data[j] != expectedPrefix[j] {
				t.Errorf("Block %d data mismatch at index %d: expected %v, got %v",
					blockNum, j, expectedPrefix[j], data[j])
			}
		}
	}
}

func TestErrorCases(t *testing.T) {
	testFileName := "test_errors.vfsl"
	defer func(name string) {
		_ = os.Remove(name)
	}(testFileName)

	disk, err := Open(testFileName, os.O_RDWR|os.O_CREATE, 0777, 4096, 100, 2, 0.75)
	if err != nil {
		t.Fatalf("Failed to open disk: %v", err)
	}

	defer func(disk *Disk) {
		_ = disk.Close()
	}(disk)

	// Test writing data larger than block size
	oversizedData := make([]byte, int(disk.header.blockSize)+1)
	_, err = disk.Write(oversizedData)
	if err == nil {
		t.Error("Expected error when writing data larger than block size, but got nil")
	}

	// Test writing to out-of-range block
	err = disk.WriteAt([]byte("Test"), disk.header.totalBlocks+1)
	if err == nil {
		t.Error("Expected error when writing to out-of-range block, but got nil")
	}

	// Test reading from out-of-range block
	_, err = disk.ReadAt(disk.header.totalBlocks + 1)
	if err == nil {
		t.Error("Expected error when reading from out-of-range block, but got nil")
	}

	// Test deleting out-of-range block
	err = disk.Delete(disk.header.totalBlocks + 1)
	if err == nil {
		t.Error("Expected error when deleting out-of-range block, but got nil")
	}
}

func TestConcurrentWrites(t *testing.T) {
	testFileName := "test_concurrent_writes.vfsl"
	defer func(name string) {
		_ = os.Remove(name)
	}(testFileName)

	disk, err := Open(testFileName, os.O_RDWR|os.O_CREATE, 0777, 4096, 1000, 2, 0.75)
	if err != nil {
		t.Fatalf("Failed to open disk: %v", err)
	}

	defer func(disk *Disk) {
		_ = disk.Close()
	}(disk)

	// Number of goroutines to use
	numGoroutines := 50
	// Number of writes per goroutine
	writesPerGoroutine := 10

	// Create a wait group to synchronize goroutines
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Channel to collect successfully written block numbers
	blockChan := make(chan uint64, numGoroutines*writesPerGoroutine)

	// Start goroutines for concurrent writing
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()

			for j := 0; j < writesPerGoroutine; j++ {
				// Create unique data for each goroutine and write
				data := []byte(fmt.Sprintf("Data from goroutine %d, write %d", id, j))
				blockNum, err := disk.Write(data)
				if err != nil {
					t.Errorf("Goroutine %d failed to write data: %v", id, err)
					continue
				}

				// Send successful block number to channel
				blockChan <- blockNum
			}
		}(i)
	}

	// Wait for all goroutines to complete
	wg.Wait()
	close(blockChan)

	// Collect all block numbers written
	writtenBlocks := make(map[uint64]bool)
	for blockNum := range blockChan {
		if writtenBlocks[blockNum] {
			t.Errorf("Block %d was allocated more than once", blockNum)
		}
		writtenBlocks[blockNum] = true
	}

	// Verify number of written blocks
	expectedBlocks := numGoroutines * writesPerGoroutine
	if len(writtenBlocks) != expectedBlocks {
		t.Errorf("Expected %d unique blocks to be written, got %d", expectedBlocks, len(writtenBlocks))
	}
}

func TestConcurrentReadWrite(t *testing.T) {
	testFileName := "test_concurrent_read_write.vfsl"
	defer func(name string) {
		_ = os.Remove(name)
	}(testFileName)

	disk, err := Open(testFileName, os.O_RDWR|os.O_CREATE, 0777, 4096, 1000, 2, 0.75)
	if err != nil {
		t.Fatalf("Failed to open disk: %v", err)
	}

	defer func(disk *Disk) {
		_ = disk.Close()
	}(disk)

	// First, write some data to specific blocks
	testBlocks := make([]uint64, 20)
	testData := make([][]byte, 20)

	for i := 0; i < 20; i++ {
		testData[i] = []byte(fmt.Sprintf("Initial data for block %d", i))
		blockNum, err := disk.Write(testData[i])
		if err != nil {
			t.Fatalf("Failed to write initial data: %v", err)
		}
		testBlocks[i] = blockNum
	}

	// Create a wait group for synchronization
	var wg sync.WaitGroup

	// Create a mutex to protect the validation
	var validationMutex sync.Mutex

	// Track test failures safely
	testErrors := make([]error, 0)
	recordError := func(err error) {
		validationMutex.Lock()
		defer validationMutex.Unlock()
		testErrors = append(testErrors, err)
	}

	// Start reader goroutines
	numReaders := 10
	wg.Add(numReaders)

	for i := 0; i < numReaders; i++ {
		go func(id int) {
			defer wg.Done()

			// Each reader reads all test blocks multiple times
			for j := 0; j < 5; j++ {
				for k, blockNum := range testBlocks {
					data, err := disk.ReadAt(blockNum)
					if err != nil {
						recordError(fmt.Errorf("Reader %d failed to read block %d: %v", id, blockNum, err))
						continue
					}

					// Convert data to string for easier pattern matching
					dataStr := string(data)

					// Check if the data matches either the original content or an updated content
					initialPattern := fmt.Sprintf("Initial data for block %d", k)

					// Create a flag to track if data matches any valid pattern
					validData := false

					// Check if it's the original data
					if strings.HasPrefix(dataStr, initialPattern) {
						validData = true
					}

					// Check if it's been updated by any writer
					for w := 0; w < 5; w++ { // Number of writers
						updatedPattern := fmt.Sprintf("Updated by writer %d", w)
						if strings.HasPrefix(dataStr, updatedPattern) {
							validData = true
							break
						}
					}

					// If data doesn't match any valid pattern, report an error
					if !validData {
						recordError(fmt.Errorf("Reader %d: unexpected data in block %d: %s",
							id, blockNum, dataStr[:20])) // Show first 20 chars
					}
				}
			}
		}(i)
	}

	// Start writer goroutines (updating data)
	numWriters := 5
	wg.Add(numWriters)

	for i := 0; i < numWriters; i++ {
		go func(id int) {
			defer wg.Done()

			// Each writer updates some blocks with new data
			for j := id; j < len(testBlocks); j += numWriters {
				blockNum := testBlocks[j]
				newData := []byte(fmt.Sprintf("Updated by writer %d", id))

				err := disk.WriteAt(newData, blockNum)
				if err != nil {
					recordError(fmt.Errorf("writer %d failed to update block %d: %v", id, blockNum, err))
				}
			}
		}(i)
	}

	// Wait for all goroutines to complete
	wg.Wait()

	// Report any errors collected during the test
	if len(testErrors) > 0 {
		for _, err := range testErrors {
			t.Error(err)
		}
	}
}

func TestConcurrentExpansion(t *testing.T) {
	testFileName := "test_concurrent_expansion.vfsl"
	defer func(name string) {
		_ = os.Remove(name)
	}(testFileName)

	// Start with a small disk that will need to expand
	initialBlocks := uint64(20)
	disk, err := Open(testFileName, os.O_RDWR|os.O_CREATE, 0777, 4096, initialBlocks, 2, 0.75)
	if err != nil {
		t.Fatalf("Failed to open disk: %v", err)
	}

	defer func(disk *Disk) {
		_ = disk.Close()
	}(disk)

	// Number of goroutines to spawn
	numGoroutines := 30 // More than initial blocks to trigger expansion

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Channel to collect results
	results := make(chan error, numGoroutines)

	// Launch goroutines that will all try to write simultaneously
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()

			// Each goroutine tries to write unique data
			data := []byte(fmt.Sprintf("Data from concurrent writer %d", id))
			_, err := disk.Write(data)
			results <- err
		}(i)
	}

	// Wait for all goroutines to finish
	wg.Wait()
	close(results)

	// Check results
	var successCount int
	for err := range results {
		if err != nil {
			t.Errorf("Concurrent write failed: %v", err)
		} else {
			successCount++
		}
	}

	// Verify all writes succeeded
	if successCount != numGoroutines {
		t.Errorf("Expected %d successful writes, got %d", numGoroutines, successCount)
	}

	// Verify disk expanded
	if disk.header.totalBlocks <= initialBlocks {
		t.Errorf("Disk did not expand as expected")
	}
}

// BenchmarkWrite tests the performance of writing data to the disk
func BenchmarkWrite(b *testing.B) {
	testFileName := "bench_write.vfsl"
	defer func(name string) {
		_ = os.Remove(name)
	}(testFileName)

	disk, err := Open(testFileName, os.O_RDWR|os.O_CREATE, 0777, 4096, 10000, 2, 0.75)
	if err != nil {
		b.Fatalf("Failed to open disk: %v", err)
	}

	defer func(disk *Disk) {
		_ = disk.Close()
	}(disk)

	// Data to write (1KB)
	data := make([]byte, 1024)
	for i := range data {
		data[i] = byte(i % 256)
	}

	// Reset the timer before starting the benchmark loops
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Write data to disk
		_, err := disk.Write(data)
		if err != nil {
			b.Fatalf("Write failed: %v", err)
		}
	}
}

// BenchmarkRead tests the performance of reading data from the disk
func BenchmarkRead(b *testing.B) {
	testFileName := "bench_read.vfsl"
	defer func(name string) {
		_ = os.Remove(name)
	}(testFileName)

	disk, err := Open(testFileName, os.O_RDWR|os.O_CREATE, 0777, 4096, 10000, 2, 0.75)
	if err != nil {
		b.Fatalf("Failed to open disk: %v", err)
	}

	defer func(disk *Disk) {
		_ = disk.Close()
	}(disk)

	// Write data first so we can read it
	data := make([]byte, 1024)
	for i := range data {
		data[i] = byte(i % 256)
	}

	// Create blocks to read
	numBlocks := 100
	blocks := make([]uint64, numBlocks)

	for i := 0; i < numBlocks; i++ {
		blockNum, err := disk.Write(data)
		if err != nil {
			b.Fatalf("Setup failed - couldn't write data: %v", err)
		}
		blocks[i] = blockNum
	}

	// Reset the timer before starting the benchmark loops
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Read a random block
		blockIndex := i % numBlocks
		_, err := disk.ReadAt(blocks[blockIndex])
		if err != nil {
			b.Fatalf("Read failed: %v", err)
		}
	}
}

// BenchmarkConcurrentWrites tests performance of concurrent writes
func BenchmarkConcurrentWrites(b *testing.B) {
	testFileName := "bench_concurrent_write.vfsl"
	defer func(name string) {
		_ = os.Remove(name)
	}(testFileName)

	disk, err := Open(testFileName, os.O_RDWR|os.O_CREATE, 0777, 4096, 10000, 2, 0.75)
	if err != nil {
		b.Fatalf("Failed to open disk: %v", err)
	}

	defer func(disk *Disk) {
		_ = disk.Close()
	}(disk)

	// Number of concurrent goroutines
	numGoroutines := 10

	// Data to write (1KB)
	data := make([]byte, 1024)
	for i := range data {
		data[i] = byte(i % 256)
	}

	// Reset timer
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Use wait group to coordinate goroutines
		var wg sync.WaitGroup
		wg.Add(numGoroutines)

		// Launch concurrent writers
		for j := 0; j < numGoroutines; j++ {
			go func() {
				defer wg.Done()
				_, err := disk.Write(data)
				if err != nil {
					b.Errorf("Concurrent write failed: %v", err)
				}
			}()
		}

		// Wait for all writes to complete
		wg.Wait()
	}
}
