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
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	"vfslite/disk"
)

// Block types
const (
	BlockTypeRoot      = 1
	BlockTypeDirectory = 2
	BlockTypeFile      = 3
	BlockTypeData      = 4
	HeaderSize         = 9    // size of Type(1) + ChildCount(2) + DataSize(4) + MetaSize(2)
	ReferenceSize      = 8    // Size of each block reference (uint64)
	DebugEnabled       = true // Set to true to enable debug output
	SuperblockNum      = 0    // Block 0 is reserved for the superblock
)

// BlockHeader defines block specific metadata at the beginning of each block
type BlockHeader struct {
	Type       uint8  // Type of block (e.g., directory, file, etc.)
	ChildCount uint16 // Number of child references
	DataSize   uint32 // Size of data in this block (excluding header and references)
	MetaSize   uint16 // Size of metadata in bytes
}

// Metadata defines common metadata for all file and directory blocks
type Metadata struct {
	CreatedOn  int64             `json:"createdOn"`  // Unix timestamp for creation time
	ModifiedOn int64             `json:"modifiedOn"` // Unix timestamp for last modification
	Extra      map[string]string `json:"extra"`      // Optional extra metadata as key-value pairs
} // JSON works fine :)

// VFSLite main structure for the virtual filesystem
type VFSLite struct {
	disk      *disk.Disk // Underlying disk structure
	blockSize uint       // Size of each block in bytes (configured by user)
	rootBlock uint64     // The entry point to our disk structure
}

// StreamWriter provides a way to write large files to the virtual disk
type StreamWriter struct {
	vfs          *VFSLite
	fileBlock    uint64 // The initial file block that contains the file metadata
	currentData  []byte // Buffer for accumulating data before writing a block
	maxBlockData uint   // Maximum data size per block (blockSize - HeaderSize - some overhead)
}

// StreamReader provides a way to read large files from the virtual disk
type StreamReader struct {
	vfs           *VFSLite
	fileBlock     uint64   // The initial file block that contains the file metadata
	dataBlocks    []uint64 // List of all data blocks for this file
	currentBlock  int      // Index of the current block being read
	currentData   []byte   // Data from the current block
	currentOffset int      // Current offset within the current block
}

// Open creates or opens a virtual disk
func Open(diskName string, blockSize uint, initialBlocks uint64) (*VFSLite, error) {
	// Open the underlying disk manager
	d, err := disk.Open(
		diskName,
		os.O_RDWR|os.O_CREATE,
		0777,
		blockSize,     // Block size
		initialBlocks, // Initial number of blocks
		2,             // Double the size when threshold is reached
		0.8,           // Increase when 80% full
	)
	if err != nil {
		return nil, fmt.Errorf("failed to open disk: %w", err)
	}

	vfs := &VFSLite{
		disk:      d,
		blockSize: blockSize,
	}

	// First, make sure block 0 is marked as allocated by writing to it
	err = d.WriteAt(make([]byte, blockSize), SuperblockNum)
	if err != nil {
		return nil, fmt.Errorf("failed to reserve superblock: %w", err)
	}

	// Try to read the superblock to see if it has a valid root pointer
	superblockData, err := d.ReadAt(SuperblockNum)

	var rootBlockNum uint64
	var needInit bool = false

	if err == nil && len(superblockData) >= 8 {
		// Extract root block pointer from superblock
		rootBlockNum = binary.LittleEndian.Uint64(superblockData[:8])

		if rootBlockNum != 0 { // Ensure root block is not the superblock or 0
			// Verify the root block is readable and has the right type
			rootData, err := d.ReadAt(rootBlockNum)
			if err != nil {
				fmt.Printf("Root block %d cannot be read, initializing new root\n", rootBlockNum)
				needInit = true
			} else if len(rootData) >= HeaderSize {
				header, err := deserializeHeader(rootData[:HeaderSize])
				if err != nil || header.Type != BlockTypeRoot {
					fmt.Printf("Block %d is not a valid root block (type=%d), initializing new root\n",
						rootBlockNum, header.Type)
					needInit = true
				} else {
					fmt.Printf("Found valid root block at block %d\n", rootBlockNum)
				}
			} else {
				needInit = true
			}
		} else {
			fmt.Println("Root block cannot be the same as superblock or zero, initializing new root")
			needInit = true
		}
	} else {
		fmt.Println("No valid superblock found, initializing new disk structure")
		needInit = true
	}

	if needInit {
		// Create a root block using Write() and let the disk manager find an available block
		// but first we need to make sure block 0 is marked as allocated
		rootData := make([]byte, HeaderSize+4) // Header + "root" data

		// Create header for root block
		header := &BlockHeader{
			Type:       BlockTypeRoot,
			ChildCount: 0,
			DataSize:   4, // "root" is 4 bytes
		}
		headerBytes, err := serializeHeader(header)
		if err != nil {
			return nil, fmt.Errorf("failed to serialize header: %w", err)
		}

		// Copy header and data into the block data
		copy(rootData[:HeaderSize], headerBytes)
		copy(rootData[HeaderSize:], []byte("root"))

		// Write the root block and let the disk manager find an available block
		rootBlockNum, err = d.Write(rootData)
		if err != nil {
			return nil, fmt.Errorf("failed to write root block: %w", err)
		}

		// Make sure we didn't get block 0
		if rootBlockNum == SuperblockNum {
			// ** This shouldn't happen since we already wrote to block 0, but just in case
			return nil, fmt.Errorf("root block was assigned to superblock, which should be reserved")
		}

		// We update superblock with pointer to root block
		superblockData = make([]byte, blockSize)
		binary.LittleEndian.PutUint64(superblockData, rootBlockNum)

		err = d.WriteAt(superblockData, SuperblockNum)
		if err != nil {
			return nil, fmt.Errorf("failed to write superblock: %w", err)
		}

		fmt.Printf("Initialized new root block at %d\n", rootBlockNum)
	}

	vfs.rootBlock = rootBlockNum
	fmt.Printf("Using root block: %d\n", vfs.rootBlock)
	return vfs, nil
}

// serializeHeader serializes a block header into bytes
func serializeHeader(header *BlockHeader) ([]byte, error) {
	buf := new(bytes.Buffer)
	err := binary.Write(buf, binary.LittleEndian, header.Type)
	if err != nil {
		return nil, err
	}

	err = binary.Write(buf, binary.LittleEndian, header.ChildCount)
	if err != nil {
		return nil, err
	}

	err = binary.Write(buf, binary.LittleEndian, header.DataSize)
	if err != nil {
		return nil, err
	}

	err = binary.Write(buf, binary.LittleEndian, header.MetaSize)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// deserializeHeader deserializes bytes into a block header
func deserializeHeader(data []byte) (*BlockHeader, error) {
	headerSize := 9 // Type(1) + ChildCount(2) + DataSize(4) + MetaSize(2)
	if len(data) < headerSize {
		return nil, errors.New("data too small for header")
	}

	header := &BlockHeader{}
	buf := bytes.NewReader(data)

	if err := binary.Read(buf, binary.LittleEndian, &header.Type); err != nil {
		return nil, fmt.Errorf("failed to read header type: %w", err)
	}

	if err := binary.Read(buf, binary.LittleEndian, &header.ChildCount); err != nil {
		return nil, fmt.Errorf("failed to read child count: %w", err)
	}

	if err := binary.Read(buf, binary.LittleEndian, &header.DataSize); err != nil {
		return nil, fmt.Errorf("failed to read data size: %w", err)
	}

	if err := binary.Read(buf, binary.LittleEndian, &header.MetaSize); err != nil {
		return nil, fmt.Errorf("failed to read metadata size: %w", err)
	}

	return header, nil
}

// createBlock allocates a new block with the given type and data
func (vfs *VFSLite) createBlock(blockType uint8, data []byte, meta *Metadata) (uint64, error) {
	if data == nil {
		data = []byte{}
	}

	// If no metadata provided, create default metadata
	if meta == nil {
		now := time.Now().Unix()
		meta = &Metadata{
			CreatedOn:  now,
			ModifiedOn: now,
			Extra:      make(map[string]string),
		}
	}

	// Serialize metadata to JSON
	metaBytes, err := json.Marshal(meta)
	if err != nil {
		return 0, fmt.Errorf("failed to serialize metadata: %w", err)
	}

	// Create header
	header := &BlockHeader{
		Type:       blockType,
		ChildCount: 0,
		DataSize:   uint32(len(data)),
		MetaSize:   uint16(len(metaBytes)),
	}

	headerBytes, err := serializeHeader(header)
	if err != nil {
		return 0, fmt.Errorf("failed to serialize header: %w", err)
	}

	// Combine header, metadata, and data
	blockData := make([]byte, len(headerBytes)+len(metaBytes)+len(data))
	copy(blockData, headerBytes)
	copy(blockData[len(headerBytes):], metaBytes)
	copy(blockData[len(headerBytes)+len(metaBytes):], data)

	// Write to disk
	blockNum, err := vfs.disk.Write(blockData)
	if err != nil {
		return 0, fmt.Errorf("failed to write block: %w", err)
	}

	// We make sure we didn't accidentally get block 0
	if blockNum == SuperblockNum {
		return 0, fmt.Errorf("block allocation returned superblock (0), which should be reserved")
	}

	return blockNum, nil
}

// readBlock reads a block and returns its header, data, and references
func (vfs *VFSLite) readBlock(blockNum uint64) (*BlockHeader, *Metadata, []byte, []uint64, error) {
	if blockNum == SuperblockNum {
		return nil, nil, nil, nil, fmt.Errorf("cannot read superblock (0) as a normal block")
	}

	blockData, err := vfs.disk.ReadAt(blockNum)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to read block %d: %w", blockNum, err)
	}

	if DebugEnabled {
		fmt.Printf("DEBUG: Reading block %d, first 16 bytes: % x\n", blockNum, blockData[:16])
	}

	headerSize := 9
	header, err := deserializeHeader(blockData[:headerSize])
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to deserialize header for block %d: %w", blockNum, err)
	}

	if DebugEnabled {
		fmt.Printf("DEBUG: Block %d header: Type=%d, ChildCount=%d, DataSize=%d, MetaSize=%d\n",
			blockNum, header.Type, header.ChildCount, header.DataSize, header.MetaSize)
	}

	// Sanity check the header values to catch corruption
	if header.ChildCount > 1000 || header.DataSize > uint32(vfs.blockSize) || header.MetaSize > 8192 {
		return nil, nil, nil, nil, fmt.Errorf("corrupted block %d: invalid header values", blockNum)
	}

	// Extract metadata
	metaStart := headerSize
	metaEnd := metaStart + int(header.MetaSize)
	if metaEnd > len(blockData) {
		return nil, nil, nil, nil, fmt.Errorf("corrupted block %d: metadata size exceeds block size", blockNum)
	}

	var meta Metadata
	if header.MetaSize > 0 {
		err = json.Unmarshal(blockData[metaStart:metaEnd], &meta)
		if err != nil {
			return nil, nil, nil, nil, fmt.Errorf("failed to deserialize metadata for block %d: %w", blockNum, err)
		}
	} else {
		// No metadata, create default
		now := time.Now().Unix()
		meta = Metadata{
			CreatedOn:  now,
			ModifiedOn: now,
			Extra:      make(map[string]string),
		}
	}

	// Extract data portion
	dataStart := metaEnd
	dataEnd := dataStart + int(header.DataSize)
	if dataEnd > len(blockData) {
		return nil, nil, nil, nil, fmt.Errorf("corrupted block %d: data size %d exceeds block size %d",
			blockNum, header.DataSize, len(blockData))
	}
	data := blockData[dataStart:dataEnd]

	// Extract references
	references := make([]uint64, header.ChildCount)
	refStart := dataEnd
	for i := 0; i < int(header.ChildCount); i++ {
		refPos := refStart + i*ReferenceSize
		if refPos+ReferenceSize > len(blockData) {
			return nil, nil, nil, nil, fmt.Errorf("corrupted block %d: references exceed block size", blockNum)
		}
		ref := binary.LittleEndian.Uint64(blockData[refPos : refPos+ReferenceSize])

		references[i] = ref
	}

	if DebugEnabled && len(references) > 0 {
		fmt.Printf("DEBUG: Block %d has %d children: %v\n", blockNum, len(references), references)
	}

	return header, &meta, data, references, nil
}

// writeBlock writes a header, data, and references to a block
func (vfs *VFSLite) writeBlock(blockNum uint64, header *BlockHeader, meta *Metadata, data []byte, references []uint64) error {
	if blockNum == SuperblockNum {
		return fmt.Errorf("cannot write to superblock (0) as a normal block")
	}

	meta.ModifiedOn = time.Now().Unix()

	metaBytes, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("failed to serialize metadata: %w", err)
	}

	// Update header with current sizes
	header.DataSize = uint32(len(data))
	header.ChildCount = uint16(len(references))
	header.MetaSize = uint16(len(metaBytes))

	headerBytes, err := serializeHeader(header)
	if err != nil {
		return fmt.Errorf("failed to serialize header: %w", err)
	}

	// Calculate total block size
	totalSize := len(headerBytes) + len(metaBytes) + len(data) + len(references)*ReferenceSize
	if uint(totalSize) > vfs.blockSize {
		return fmt.Errorf("data too large for block: %d > %d", totalSize, vfs.blockSize)
	}

	// Combine header, metadata, data, and references
	blockData := make([]byte, totalSize)

	// Copy header
	copy(blockData, headerBytes)

	// Copy metadata
	copy(blockData[len(headerBytes):], metaBytes)

	// Copy data
	copy(blockData[len(headerBytes)+len(metaBytes):], data)

	// Copy references
	refStart := len(headerBytes) + len(metaBytes) + len(data)
	for i, ref := range references {
		refPos := refStart + i*ReferenceSize
		binary.LittleEndian.PutUint64(blockData[refPos:refPos+ReferenceSize], ref)
	}

	// Write to disk
	err = vfs.disk.WriteAt(blockData, blockNum)
	if err != nil {
		return fmt.Errorf("failed to write block %d: %w", blockNum, err)
	}

	return nil
}

// AddChild adds a child reference to a parent block
func (vfs *VFSLite) AddChild(parentBlock uint64, childBlock uint64) error {
	if parentBlock == SuperblockNum || childBlock == SuperblockNum {
		return fmt.Errorf("cannot use superblock (0) in parent-child relationship")
	}

	// Read the parent block
	header, meta, data, references, err := vfs.readBlock(parentBlock)
	if err != nil {
		return err
	}

	// Add the new reference
	newReferences := append(references, childBlock)

	// Write back the block with the new reference
	return vfs.writeBlock(parentBlock, header, meta, data, newReferences)
}

// CreateFileBlock creates a new file block and adds it as a child of the given parent
func (vfs *VFSLite) CreateFileBlock(parentBlock uint64, fileName string, meta *Metadata) (uint64, error) {
	if parentBlock == SuperblockNum {
		return 0, fmt.Errorf("cannot use superblock (0) as parent")
	}

	// Create a file block with the file name as data and the provided metadata
	fileBlock, err := vfs.createBlock(BlockTypeFile, []byte(fileName), meta)
	if err != nil {
		return 0, err
	}

	// Add it as a child to the parent
	err = vfs.AddChild(parentBlock, fileBlock)
	if err != nil {
		// Clean up on failure
		err = vfs.disk.Delete(fileBlock)
		if err != nil {
			return 0, err
		}
		return 0, err
	}

	return fileBlock, nil
}

// AppendData appends data to a file by creating a data block and linking it
func (vfs *VFSLite) AppendData(fileBlock uint64, data []byte) error {
	if fileBlock == SuperblockNum {
		return fmt.Errorf("cannot append data to superblock (0)")
	}

	// Create a data block
	dataBlock, err := vfs.createBlock(BlockTypeData, data, nil)
	if err != nil {
		return err
	}

	// Add the data block as a child of the file block
	return vfs.AddChild(fileBlock, dataBlock)
}

// ListChildren returns all children of a block
func (vfs *VFSLite) ListChildren(blockNum uint64) ([]uint64, error) {
	if blockNum == SuperblockNum {
		return nil, fmt.Errorf("cannot list children of superblock (0)")
	}

	_, _, _, references, err := vfs.readBlock(blockNum)
	return references, err
}

// GetRootBlock returns the root block number
func (vfs *VFSLite) GetRootBlock() uint64 {
	return vfs.rootBlock
}

// CreateDirectoryBlock creates a directory block and adds it as a child of the parent
func (vfs *VFSLite) CreateDirectoryBlock(parentBlock uint64, dirName string, meta *Metadata) (uint64, error) {
	if parentBlock == SuperblockNum {
		return 0, fmt.Errorf("cannot use superblock (0) as parent")
	}

	// Create a directory block with the directory name as data and the provided metadata
	dirBlock, err := vfs.createBlock(BlockTypeDirectory, []byte(dirName), meta)
	if err != nil {
		return 0, err
	}

	// Add it as a child to the parent
	err = vfs.AddChild(parentBlock, dirBlock)
	if err != nil {
		// Clean up on failure
		err = vfs.disk.Delete(dirBlock)
		if err != nil {
			return 0, err
		}
		return 0, err
	}

	return dirBlock, nil
}

// GetBlockMetadata returns the metadata for a block
func (vfs *VFSLite) GetBlockMetadata(blockNum uint64) (*Metadata, error) {
	if blockNum == SuperblockNum {
		return nil, fmt.Errorf("cannot get metadata from superblock (0)")
	}

	_, meta, _, _, err := vfs.readBlock(blockNum)
	return meta, err
}

// UpdateBlockMetadata updates the metadata for a block
func (vfs *VFSLite) UpdateBlockMetadata(blockNum uint64, updateFn func(*Metadata)) error {
	if blockNum == SuperblockNum {
		return fmt.Errorf("cannot update metadata for superblock (0)")
	}

	header, meta, data, references, err := vfs.readBlock(blockNum)
	if err != nil {
		return err
	}

	// Apply the update function to modify the metadata
	updateFn(meta)

	// Write back the block with updated metadata
	return vfs.writeBlock(blockNum, header, meta, data, references)
}

// SetExtraMetadata sets an extra metadata key-value pair for a block
func (vfs *VFSLite) SetExtraMetadata(blockNum uint64, key, value string) error {
	return vfs.UpdateBlockMetadata(blockNum, func(meta *Metadata) {
		if meta.Extra == nil {
			meta.Extra = make(map[string]string)
		}
		meta.Extra[key] = value
	})
}

// GetExtraMetadata gets an extra metadata value for a block
func (vfs *VFSLite) GetExtraMetadata(blockNum uint64, key string) (string, bool, error) {
	meta, err := vfs.GetBlockMetadata(blockNum)
	if err != nil {
		return "", false, err
	}

	if meta.Extra == nil {
		return "", false, nil
	}

	value, exists := meta.Extra[key]
	return value, exists, nil
}

// GetBlockData returns the data stored in a block
func (vfs *VFSLite) GetBlockData(blockNum uint64) ([]byte, error) {
	if blockNum == SuperblockNum {
		return nil, fmt.Errorf("cannot get data from superblock (0)")
	}

	_, _, data, _, err := vfs.readBlock(blockNum)
	return data, err
}

// GetBlockType returns the type of a block
func (vfs *VFSLite) GetBlockType(blockNum uint64) (uint8, error) {
	if blockNum == SuperblockNum {
		return 0, fmt.Errorf("cannot get type of superblock (0)")
	}

	header, _, _, _, err := vfs.readBlock(blockNum)
	if err != nil {
		return 0, err
	}
	return header.Type, nil
}

// Close closes the virtual disk
func (vfs *VFSLite) Close() error {
	// Ensure any pending writes are flushed to disk
	if vfs.disk != nil && vfs.disk.File() != nil {
		_ = vfs.disk.File().Sync()
	}
	return vfs.disk.Close()
}

// CreateStreamWriter creates a new writer for streaming data to a file
func (vfs *VFSLite) CreateStreamWriter(parentBlock uint64, fileName string) (*StreamWriter, error) {
	// Create a file block to hold metadata
	fileBlock, err := vfs.CreateFileBlock(parentBlock, fileName, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create file block: %w", err)
	}

	// Calculate maximum data size per block
	// Allow some space for header and potential future metadata
	maxBlockData := vfs.blockSize - HeaderSize - 64 // 64 bytes for future expansion

	return &StreamWriter{
		vfs:          vfs,
		fileBlock:    fileBlock,
		currentData:  make([]byte, 0, maxBlockData),
		maxBlockData: uint(maxBlockData),
	}, nil
}

// Write adds data to the stream, automatically creating blocks as needed
func (sw *StreamWriter) Write(data []byte) (int, error) {
	totalWritten := 0

	for len(data) > 0 {
		// Calculate how much data we can add to the current buffer
		remainingSpace := int(sw.maxBlockData) - len(sw.currentData)
		chunkSize := len(data)
		if chunkSize > remainingSpace {
			chunkSize = remainingSpace
		}

		// Add data to the buffer
		sw.currentData = append(sw.currentData, data[:chunkSize]...)
		data = data[chunkSize:]
		totalWritten += chunkSize

		// If buffer is full, write it to a block
		if len(sw.currentData) >= int(sw.maxBlockData) {
			if err := sw.flushBlock(); err != nil {
				return totalWritten, err
			}
		}
	}

	return totalWritten, nil
}

// flushBlock writes the current buffer to a new data block
func (sw *StreamWriter) flushBlock() error {
	if len(sw.currentData) == 0 {
		return nil // Nothing to flush
	}

	// Create a data block with the buffered data
	err := sw.vfs.AppendData(sw.fileBlock, sw.currentData)
	if err != nil {
		return fmt.Errorf("failed to write data block: %w", err)
	}

	// Reset the buffer
	sw.currentData = make([]byte, 0, sw.maxBlockData)

	return nil
}

// Close finalizes the stream, writing any remaining data
func (sw *StreamWriter) Close() error {
	// Write any remaining data in the buffer
	if len(sw.currentData) > 0 {
		if err := sw.flushBlock(); err != nil {
			return err
		}
	}

	return nil
}

// OpenStreamReader creates a reader for streaming data from a file
func (vfs *VFSLite) OpenStreamReader(fileBlock uint64) (*StreamReader, error) {
	// Verify this is a file block
	blockType, err := vfs.GetBlockType(fileBlock)
	if err != nil {
		return nil, fmt.Errorf("error getting block type: %w", err)
	}

	if blockType != BlockTypeFile {
		return nil, fmt.Errorf("block %d is not a file block", fileBlock)
	}

	// Get all data blocks for this file
	dataBlocks, err := vfs.ListChildren(fileBlock)
	if err != nil {
		return nil, fmt.Errorf("failed to list data blocks: %w", err)
	}

	return &StreamReader{
		vfs:           vfs,
		fileBlock:     fileBlock,
		dataBlocks:    dataBlocks,
		currentBlock:  -1, // No block loaded yet
		currentData:   nil,
		currentOffset: 0,
	}, nil
}

// Read reads data from the stream into the provided buffer
func (sr *StreamReader) Read(buffer []byte) (int, error) {
	totalRead := 0

	for totalRead < len(buffer) {
		// If we don't have a current block or have finished the current block, load the next one
		if sr.currentBlock == -1 || sr.currentOffset >= len(sr.currentData) {
			// Move to the next block
			sr.currentBlock++

			// If we've read all blocks, return EOF
			if sr.currentBlock >= len(sr.dataBlocks) {
				if totalRead == 0 {
					return 0, io.EOF
				}
				return totalRead, nil
			}

			// Load data from the current block
			data, err := sr.vfs.GetBlockData(sr.dataBlocks[sr.currentBlock])
			if err != nil {
				return totalRead, fmt.Errorf("failed to read data block %d: %w", sr.dataBlocks[sr.currentBlock], err)
			}

			sr.currentData = data
			sr.currentOffset = 0
		}

		// Copy data from the current block to the buffer
		remaining := len(sr.currentData) - sr.currentOffset
		copySize := len(buffer) - totalRead
		if copySize > remaining {
			copySize = remaining
		}

		copy(buffer[totalRead:totalRead+copySize], sr.currentData[sr.currentOffset:sr.currentOffset+copySize])

		sr.currentOffset += copySize
		totalRead += copySize
	}

	return totalRead, nil
}

// Close closes the stream reader
func (sr *StreamReader) Close() error {
	// Reset all fields
	sr.currentBlock = -1
	sr.currentData = nil
	sr.currentOffset = 0

	return nil
}

// GetFileSize returns the total size of a file by summing all its data blocks
func (vfs *VFSLite) GetFileSize(fileBlock uint64) (uint64, error) {
	// Verify this is a file block
	blockType, err := vfs.GetBlockType(fileBlock)
	if err != nil {
		return 0, fmt.Errorf("error getting block type: %w", err)
	}

	if blockType != BlockTypeFile {
		return 0, fmt.Errorf("block %d is not a file block", fileBlock)
	}

	// Get all data blocks for this file
	dataBlocks, err := vfs.ListChildren(fileBlock)
	if err != nil {
		return 0, fmt.Errorf("failed to list data blocks: %w", err)
	}

	var totalSize uint64

	// Sum up the sizes of all data blocks
	for _, blockNum := range dataBlocks {
		header, _, _, _, err := vfs.readBlock(blockNum)
		if err != nil {
			return 0, fmt.Errorf("failed to read block %d: %w", blockNum, err)
		}

		totalSize += uint64(header.DataSize)
	}

	return totalSize, nil
}

// WriteFile writes an entire file to the virtual disk
// For large files, using StreamWriter directly is more memory efficient
func (vfs *VFSLite) WriteFile(parentDir uint64, fileName string, data []byte) (uint64, error) {
	writer, err := vfs.CreateStreamWriter(parentDir, fileName)
	if err != nil {
		return 0, err
	}

	_, err = writer.Write(data)
	if err != nil {
		return 0, err
	}

	err = writer.Close()
	if err != nil {
		return 0, err
	}

	return writer.fileBlock, nil
}

// ReadFile reads an entire file from the virtual disk
// For large files, using StreamReader directly is more memory efficient
func (vfs *VFSLite) ReadFile(fileBlock uint64) ([]byte, error) {
	// Get the file size first
	size, err := vfs.GetFileSize(fileBlock)
	if err != nil {
		return nil, err
	}

	// Create a buffer to hold the entire file
	data := make([]byte, size)

	reader, err := vfs.OpenStreamReader(fileBlock)
	if err != nil {
		return nil, err
	}
	defer func(reader *StreamReader) {
		_ = reader.Close()

	}(reader)

	// Read the entire file
	_, err = reader.Read(data)
	if err != nil && err != io.EOF {
		return nil, err
	}

	return data, nil
}

// GetDirectory finds a directory by path from a starting directory
// Path segments should be separated by '/' and can be either absolute (starting with '/')
// or relative to the provided startDir. Supports ".." for parent directory navigation.
func (vfs *VFSLite) GetDirectory(startDir uint64, path string) (uint64, error) {
	// If path starts with '/', start from root
	if strings.HasPrefix(path, "/") {
		startDir = vfs.rootBlock
		path = path[1:] // Remove leading '/'
	}

	// If path is empty or ".", return the starting directory
	if path == "" || path == "." {
		return startDir, nil
	}

	// Split path into components
	components := strings.Split(path, "/")
	currentDir := startDir

	// We'll need to maintain a stack of parent directories
	parentStack := []uint64{startDir}

	for _, component := range components {
		// Skip empty components (e.g., from "//" in the path)
		if component == "" {
			continue
		}

		// Handle ".." (parent directory)
		if component == ".." {
			// Make sure we have a parent to go back to
			if len(parentStack) <= 1 {
				return 0, fmt.Errorf("cannot go up from this directory")
			}

			// Pop the current directory and go back to parent
			parentStack = parentStack[:len(parentStack)-1]
			currentDir = parentStack[len(parentStack)-1]
			continue
		}

		// Handle "." (current directory) - just skip it
		if component == "." {
			continue
		}

		// Get children of the current directory
		children, err := vfs.ListChildren(currentDir)
		if err != nil {
			return 0, fmt.Errorf("error listing children: %w", err)
		}

		// Look for matching directory name
		found := false
		for _, childBlock := range children {

			// Check if this is a directory..
			blockType, err := vfs.GetBlockType(childBlock)
			if err != nil {
				return 0, fmt.Errorf("error getting block type: %w", err)
			}

			if blockType == BlockTypeDirectory {
				// Get the directory name
				nameBytes, err := vfs.GetBlockData(childBlock)
				if err != nil {
					return 0, fmt.Errorf("error getting block data: %w", err)
				}

				name := string(nameBytes)
				if name == component {
					// Found the directory, continue to the next path component
					currentDir = childBlock
					parentStack = append(parentStack, currentDir) // Keep track of this directory
					found = true
					break
				}
			}
		}

		if !found {
			return 0, fmt.Errorf("directory not found: %s", component)
		}
	}

	return currentDir, nil
}

// GetFile finds a file by name within a directory
func (vfs *VFSLite) GetFile(dirBlock uint64, fileName string) (uint64, error) {
	// Get children of the directory
	children, err := vfs.ListChildren(dirBlock)
	if err != nil {
		return 0, fmt.Errorf("error listing children: %w", err)
	}

	// Look for matching file name
	for _, childBlock := range children {
		// Check if this is a file
		blockType, err := vfs.GetBlockType(childBlock)
		if err != nil {
			return 0, fmt.Errorf("error getting block type: %w", err)
		}

		if blockType == BlockTypeFile {
			// Get the file name
			nameBytes, err := vfs.GetBlockData(childBlock)
			if err != nil {
				return 0, fmt.Errorf("error getting block data: %w", err)
			}

			name := string(nameBytes)
			if name == fileName {
				return childBlock, nil
			}
		}
	}

	return 0, fmt.Errorf("file not found: %s", fileName)
}

// GetFileByPath finds a file by path from a starting directory
func (vfs *VFSLite) GetFileByPath(startDir uint64, path string) (uint64, error) {
	// Split the path into directory path and filename
	lastSlash := strings.LastIndex(path, "/")
	var dirPath, fileName string

	if lastSlash == -1 {
		// No slashes, file is in the starting directory
		dirPath = ""
		fileName = path
	} else {
		dirPath = path[:lastSlash]
		fileName = path[lastSlash+1:]
	}

	// First get the directory
	dirBlock, err := vfs.GetDirectory(startDir, dirPath)
	if err != nil {
		return 0, err
	}

	// Then get the file in that directory
	return vfs.GetFile(dirBlock, fileName)
}

// DeleteFile removes a file and all its data blocks
func (vfs *VFSLite) DeleteFile(fileBlock uint64) error {
	if fileBlock == SuperblockNum {
		return fmt.Errorf("cannot delete superblock (0)")
	}

	// First, verify this is a file block
	blockType, err := vfs.GetBlockType(fileBlock)
	if err != nil {
		return fmt.Errorf("error getting block type: %w", err)
	}

	if blockType != BlockTypeFile {
		return fmt.Errorf("block %d is not a file block", fileBlock)
	}

	// Get all data blocks for this file
	dataBlocks, err := vfs.ListChildren(fileBlock)
	if err != nil {
		return fmt.Errorf("failed to list data blocks: %w", err)
	}

	// Delete all data blocks
	for _, blockNum := range dataBlocks {
		if err := vfs.disk.Delete(blockNum); err != nil {
			return fmt.Errorf("failed to delete data block %d: %w", blockNum, err)
		}
	}

	// Delete the file block itself
	if err := vfs.disk.Delete(fileBlock); err != nil {
		return fmt.Errorf("failed to delete file block %d: %w", fileBlock, err)
	}

	return nil
}

// DeleteFileFromParent removes a file from its parent directory and deletes the file
func (vfs *VFSLite) DeleteFileFromParent(parentDir uint64, fileName string) error {
	if parentDir == SuperblockNum {
		return fmt.Errorf("cannot use superblock (0) as parent directory")
	}

	// First, get the file block
	fileBlock, err := vfs.GetFile(parentDir, fileName)
	if err != nil {
		return err
	}

	// Remove the file from the parent directory's children
	if err := vfs.RemoveChild(parentDir, fileBlock); err != nil {
		return fmt.Errorf("failed to remove file reference from parent: %w", err)
	}

	// Now delete the file and its data blocks
	return vfs.DeleteFile(fileBlock)
}

// RemoveChild removes a child reference from a parent block
func (vfs *VFSLite) RemoveChild(parentBlock uint64, childBlock uint64) error {
	if parentBlock == SuperblockNum || childBlock == SuperblockNum {
		return fmt.Errorf("cannot use superblock (0) in parent-child relationship")
	}

	// Read the parent block
	header, meta, data, references, err := vfs.readBlock(parentBlock)
	if err != nil {
		return err
	}

	// Find and remove the child reference
	found := false
	newReferences := make([]uint64, 0, len(references))
	for _, ref := range references {
		if ref != childBlock {
			newReferences = append(newReferences, ref)
		} else {
			found = true
		}
	}

	if !found {
		return fmt.Errorf("child block %d not found in parent %d", childBlock, parentBlock)
	}

	// Write back the block with the updated references
	return vfs.writeBlock(parentBlock, header, meta, data, newReferences)
}

// DeleteDirectory removes a directory and all its contents recursively
func (vfs *VFSLite) DeleteDirectory(dirBlock uint64) error {
	if dirBlock == SuperblockNum {
		return fmt.Errorf("cannot delete superblock (0)")
	}

	// First, verify this is a directory block
	blockType, err := vfs.GetBlockType(dirBlock)
	if err != nil {
		return fmt.Errorf("error getting block type: %w", err)
	}

	if blockType != BlockTypeDirectory {
		return fmt.Errorf("block %d is not a directory block", dirBlock)
	}

	// Get all children of this directory
	children, err := vfs.ListChildren(dirBlock)
	if err != nil {
		return fmt.Errorf("failed to list children: %w", err)
	}

	// Recursively delete all children
	for _, childBlock := range children {
		childType, err := vfs.GetBlockType(childBlock)
		if err != nil {
			return fmt.Errorf("error getting block type for child %d: %w", childBlock, err)
		}

		if childType == BlockTypeDirectory {
			// Recursively delete subdirectory
			if err := vfs.DeleteDirectory(childBlock); err != nil {
				return err
			}
		} else if childType == BlockTypeFile {
			// Delete file and its data blocks
			if err := vfs.DeleteFile(childBlock); err != nil {
				return err
			}
		} else {
			// Unknown block type, just delete it directly
			if err := vfs.disk.Delete(childBlock); err != nil {
				return fmt.Errorf("failed to delete block %d: %w", childBlock, err)
			}
		}
	}

	// Delete the directory block itself
	if err := vfs.disk.Delete(dirBlock); err != nil {
		return fmt.Errorf("failed to delete directory block %d: %w", dirBlock, err)
	}

	return nil
}

// DeleteDirectoryFromParent removes a directory from its parent and deletes it recursively
func (vfs *VFSLite) DeleteDirectoryFromParent(parentDir uint64, dirName string) error {
	if parentDir == SuperblockNum {
		return fmt.Errorf("cannot use superblock (0) as parent directory")
	}

	// Get children of the directory
	children, err := vfs.ListChildren(parentDir)
	if err != nil {
		return fmt.Errorf("error listing children: %w", err)
	}

	// Look for matching directory name
	var dirBlock uint64
	dirFound := false

	for _, childBlock := range children {
		// Check if this is a directory
		blockType, err := vfs.GetBlockType(childBlock)
		if err != nil {
			return fmt.Errorf("error getting block type: %w", err)
		}

		if blockType == BlockTypeDirectory {
			// Get the directory name
			nameBytes, err := vfs.GetBlockData(childBlock)
			if err != nil {
				return fmt.Errorf("error getting block data: %w", err)
			}

			name := string(nameBytes)
			if name == dirName {
				dirBlock = childBlock
				dirFound = true
				break
			}
		}
	}

	if !dirFound {
		return fmt.Errorf("directory not found: %s", dirName)
	}

	// Remove the directory from the parent's children
	if err := vfs.RemoveChild(parentDir, dirBlock); err != nil {
		return fmt.Errorf("failed to remove directory reference from parent: %w", err)
	}

	// Now delete the directory and all its contents
	return vfs.DeleteDirectory(dirBlock)
}

// DeleteByPath deletes a file or directory at the specified path
func (vfs *VFSLite) DeleteByPath(startDir uint64, path string) error {
	// Split the path into directory path and name
	lastSlash := strings.LastIndex(path, "/")
	var dirPath, name string

	if lastSlash == -1 {
		// No slashes, item is in the starting directory
		dirPath = ""
		name = path
	} else {
		dirPath = path[:lastSlash]
		name = path[lastSlash+1:]
	}

	// First get the parent directory
	parentDir, err := vfs.GetDirectory(startDir, dirPath)
	if err != nil {
		return err
	}

	// Try to find it as a file first
	_, err = vfs.GetFile(parentDir, name)
	if err == nil {
		// It's a file, delete it
		return vfs.DeleteFileFromParent(parentDir, name)
	}

	// Try to find it as a directory
	children, err := vfs.ListChildren(parentDir)
	if err != nil {
		return fmt.Errorf("error listing children: %w", err)
	}

	for _, childBlock := range children {
		blockType, err := vfs.GetBlockType(childBlock)
		if err != nil {
			return fmt.Errorf("error getting block type: %w", err)
		}

		if blockType == BlockTypeDirectory {
			nameBytes, err := vfs.GetBlockData(childBlock)
			if err != nil {
				return fmt.Errorf("error getting block data: %w", err)
			}

			if string(nameBytes) == name {
				// It's a directory, delete it
				return vfs.DeleteDirectoryFromParent(parentDir, name)
			}
		}
	}

	return fmt.Errorf("no file or directory found at path: %s", path)
}

// RenameBlock updates the name of a file or directory block
func (vfs *VFSLite) RenameBlock(blockNum uint64, newName string) error {
	if blockNum == SuperblockNum {
		return fmt.Errorf("cannot rename superblock (0)")
	}

	// Read the block
	header, meta, _, references, err := vfs.readBlock(blockNum)
	if err != nil {
		return fmt.Errorf("failed to read block %d: %w", blockNum, err)
	}

	// Verify this is a file or directory block
	if header.Type != BlockTypeFile && header.Type != BlockTypeDirectory {
		return fmt.Errorf("block %d is not a file or directory block", blockNum)
	}

	// Update the block with the new name
	err = vfs.writeBlock(blockNum, header, meta, []byte(newName), references)
	if err != nil {
		return fmt.Errorf("failed to write block %d: %w", blockNum, err)
	}

	return nil
}

// RenameFile renames a file in a directory
func (vfs *VFSLite) RenameFile(dirBlock uint64, oldName, newName string) error {
	if dirBlock == SuperblockNum {
		return fmt.Errorf("cannot use superblock (0) as directory")
	}

	// First, get the file block
	fileBlock, err := vfs.GetFile(dirBlock, oldName)
	if err != nil {
		return fmt.Errorf("file not found: %s - %w", oldName, err)
	}

	// Verify the new name doesn't already exist
	_, err = vfs.GetFile(dirBlock, newName)
	if err == nil {
		return fmt.Errorf("file already exists: %s", newName)
	}

	// Rename the file block
	return vfs.RenameBlock(fileBlock, newName)
}

// RenameDirectory renames a directory in a parent directory
func (vfs *VFSLite) RenameDirectory(parentDir uint64, oldName, newName string) error {
	if parentDir == SuperblockNum {
		return fmt.Errorf("cannot use superblock (0) as parent directory")
	}

	// First, verify the directory exists
	children, err := vfs.ListChildren(parentDir)
	if err != nil {
		return fmt.Errorf("error listing children: %w", err)
	}

	var dirBlock uint64
	dirFound := false

	// Find the directory with the old name
	for _, childBlock := range children {
		blockType, err := vfs.GetBlockType(childBlock)
		if err != nil {
			return fmt.Errorf("error getting block type: %w", err)
		}

		if blockType == BlockTypeDirectory {
			nameBytes, err := vfs.GetBlockData(childBlock)
			if err != nil {
				return fmt.Errorf("error getting block data: %w", err)
			}

			if string(nameBytes) == oldName {
				dirBlock = childBlock
				dirFound = true
				break
			}
		}
	}

	if !dirFound {
		return fmt.Errorf("directory not found: %s", oldName)
	}

	// Verify the new name doesn't already exist
	for _, childBlock := range children {
		if childBlock == dirBlock {
			continue // Skip the directory we're renaming
		}

		blockType, err := vfs.GetBlockType(childBlock)
		if err != nil {
			return fmt.Errorf("error getting block type: %w", err)
		}

		if blockType == BlockTypeDirectory {
			nameBytes, err := vfs.GetBlockData(childBlock)
			if err != nil {
				return fmt.Errorf("error getting block data: %w", err)
			}

			if string(nameBytes) == newName {
				return fmt.Errorf("directory already exists: %s", newName)
			}
		}
	}

	// Rename the directory block
	return vfs.RenameBlock(dirBlock, newName)
}

// RenameByPath renames a file or directory at the specified path
func (vfs *VFSLite) RenameByPath(startDir uint64, path string, newName string) error {
	// Split the path into directory path and name
	lastSlash := strings.LastIndex(path, "/")
	var dirPath, name string

	if lastSlash == -1 {
		// No slashes, item is in the starting directory
		dirPath = ""
		name = path
	} else {
		dirPath = path[:lastSlash]
		name = path[lastSlash+1:]
	}

	// First get the parent directory
	parentDir, err := vfs.GetDirectory(startDir, dirPath)
	if err != nil {
		return err
	}

	// Try to find it as a file first
	_, err = vfs.GetFile(parentDir, name)
	if err == nil {
		// It's a file, rename it
		return vfs.RenameFile(parentDir, name, newName)
	}

	// Try to find it as a directory
	children, err := vfs.ListChildren(parentDir)
	if err != nil {
		return fmt.Errorf("error listing children: %w", err)
	}

	for _, childBlock := range children {
		blockType, err := vfs.GetBlockType(childBlock)
		if err != nil {
			return fmt.Errorf("error getting block type: %w", err)
		}

		if blockType == BlockTypeDirectory {
			nameBytes, err := vfs.GetBlockData(childBlock)
			if err != nil {
				return fmt.Errorf("error getting block data: %w", err)
			}

			if string(nameBytes) == name {
				// It's a directory, rename it
				return vfs.RenameDirectory(parentDir, name, newName)
			}
		}
	}

	return fmt.Errorf("no file or directory found at path: %s", path)
}
