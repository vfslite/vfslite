// Package vfslite C
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
package main

import (
	"C"
	"encoding/json"
	"fmt"
	"sync"
	"unsafe"
	"vfslite"
)

// Store VFSLite instances in a map with a mutex for thread safety
var (
	vfsMap    = make(map[int]*vfslite.VFSLite)
	writerMap = make(map[int]*vfslite.StreamWriter)
	readerMap = make(map[int]*vfslite.StreamReader)
	vfsMutex  = &sync.RWMutex{}
	nextID    = 1
)

// getNextID returns the next available ID for a VFSLite instance
func getNextID() int {
	vfsMutex.Lock()
	defer vfsMutex.Unlock()
	id := nextID
	nextID++
	return id
}

// storeVFS stores a VFSLite instance and returns its ID
func storeVFS(vfs *vfslite.VFSLite) int {
	vfsMutex.Lock()
	defer vfsMutex.Unlock()
	id := nextID
	vfsMap[id] = vfs
	nextID++
	return id
}

// getVFS retrieves a VFSLite instance by ID
func getVFS(id int) *vfslite.VFSLite {
	vfsMutex.RLock()
	defer vfsMutex.RUnlock()
	return vfsMap[id]
}

// removeVFS removes a VFSLite instance from the map
func removeVFS(id int) {
	vfsMutex.Lock()
	defer vfsMutex.Unlock()
	delete(vfsMap, id)
}

// storeWriter stores a StreamWriter instance and returns its ID
func storeWriter(writer *vfslite.StreamWriter) int {
	vfsMutex.Lock()
	defer vfsMutex.Unlock()
	id := nextID
	writerMap[id] = writer
	nextID++
	return id
}

// getWriter retrieves a StreamWriter instance by ID
func getWriter(id int) *vfslite.StreamWriter {
	vfsMutex.RLock()
	defer vfsMutex.RUnlock()
	return writerMap[id]
}

// removeWriter removes a StreamWriter instance from the map
func removeWriter(id int) {
	vfsMutex.Lock()
	defer vfsMutex.Unlock()
	delete(writerMap, id)
}

// storeReader stores a StreamReader instance and returns its ID
func storeReader(reader *vfslite.StreamReader) int {
	vfsMutex.Lock()
	defer vfsMutex.Unlock()
	id := nextID
	readerMap[id] = reader
	nextID++
	return id
}

// getReader retrieves a StreamReader instance by ID
func getReader(id int) *vfslite.StreamReader {
	vfsMutex.RLock()
	defer vfsMutex.RUnlock()
	return readerMap[id]
}

// removeReader removes a StreamReader instance from the map
func removeReader(id int) {
	vfsMutex.Lock()
	defer vfsMutex.Unlock()
	delete(readerMap, id)
}

// C API functions

//export vfsl_open
func vfsl_open(diskName *C.char, blockSize C.uint, initialBlocks C.ulonglong) C.int {
	goDiskName := C.GoString(diskName)
	goBlockSize := uint(blockSize)
	goInitialBlocks := uint64(initialBlocks)

	vfs, err := vfslite.Open(goDiskName, goBlockSize, goInitialBlocks)
	if err != nil {
		// Return negative value to indicate error
		fmt.Printf("Error opening VFSLite: %v\n", err)
		return C.int(-1)
	}

	id := storeVFS(vfs)
	return C.int(id)
}

//export vfsl_close
func vfsl_close(handle C.int) C.int {
	vfs := getVFS(int(handle))
	if vfs == nil {
		return C.int(-1) // Invalid handle
	}

	err := vfs.Close()
	removeVFS(int(handle))

	if err != nil {
		return C.int(-2) // Error closing
	}

	return C.int(0) // Success
}

//export vfsl_get_root_block
func vfsl_get_root_block(handle C.int) C.ulonglong {
	vfs := getVFS(int(handle))
	if vfs == nil {
		return 0 // Invalid handle or error
	}

	return C.ulonglong(vfs.GetRootBlock())
}

//export vfsl_create_directory_block
func vfsl_create_directory_block(handle C.int, parentBlock C.ulonglong, dirName *C.char) C.ulonglong {
	vfs := getVFS(int(handle))
	if vfs == nil {
		return 0 // Invalid handle
	}

	goDirName := C.GoString(dirName)
	now := vfslite.Metadata{} // Use default metadata

	dirBlock, err := vfs.CreateDirectoryBlock(uint64(parentBlock), goDirName, &now)
	if err != nil {
		fmt.Printf("Error creating directory block: %v\n", err)
		return 0 // Error
	}

	return C.ulonglong(dirBlock)
}

//export vfsl_create_file_block
func vfsl_create_file_block(handle C.int, parentBlock C.ulonglong, fileName *C.char) C.ulonglong {
	vfs := getVFS(int(handle))
	if vfs == nil {
		return 0 // Invalid handle
	}

	goFileName := C.GoString(fileName)
	now := vfslite.Metadata{} // Use default metadata

	fileBlock, err := vfs.CreateFileBlock(uint64(parentBlock), goFileName, &now)
	if err != nil {
		fmt.Printf("Error creating file block: %v\n", err)
		return 0 // Error
	}

	return C.ulonglong(fileBlock)
}

//export vfsl_write_file
func vfsl_write_file(handle C.int, parentDir C.ulonglong, fileName *C.char, data unsafe.Pointer, dataSize C.size_t) C.ulonglong {
	vfs := getVFS(int(handle))
	if vfs == nil {
		return 0 // Invalid handle
	}

	goFileName := C.GoString(fileName)
	goData := C.GoBytes(data, C.int(dataSize))

	fileBlock, err := vfs.WriteFile(uint64(parentDir), goFileName, goData)
	if err != nil {
		fmt.Printf("Error writing file: %v\n", err)
		return 0 // Error
	}

	return C.ulonglong(fileBlock)
}

//export vfsl_read_file
func vfsl_read_file(handle C.int, fileBlock C.ulonglong, buffer unsafe.Pointer, bufferSize C.size_t) C.size_t {
	vfs := getVFS(int(handle))
	if vfs == nil {
		return 0 // Invalid handle
	}

	data, err := vfs.ReadFile(uint64(fileBlock))
	if err != nil {
		fmt.Printf("Error reading file: %v\n", err)
		return 0 // Error
	}

	// Make sure we don't overflow the buffer
	copyLen := len(data)
	if int(bufferSize) < copyLen {
		copyLen = int(bufferSize)
	}

	// Copy the data to the provided buffer
	dst := (*[1 << 30]byte)(buffer)[:copyLen]
	copy(dst, data[:copyLen])

	return C.size_t(copyLen)
}

//export vfsl_get_file_size
func vfsl_get_file_size(handle C.int, fileBlock C.ulonglong) C.ulonglong {
	vfs := getVFS(int(handle))
	if vfs == nil {
		return 0 // Invalid handle
	}

	size, err := vfs.GetFileSize(uint64(fileBlock))
	if err != nil {
		fmt.Printf("Error getting file size: %v\n", err)
		return 0 // Error
	}

	return C.ulonglong(size)
}

//export vfsl_get_directory
func vfsl_get_directory(handle C.int, startDir C.ulonglong, path *C.char) C.ulonglong {
	vfs := getVFS(int(handle))
	if vfs == nil {
		return 0 // Invalid handle
	}

	goPath := C.GoString(path)

	dirBlock, err := vfs.GetDirectory(uint64(startDir), goPath)
	if err != nil {
		fmt.Printf("Error getting directory: %v\n", err)
		return 0 // Error
	}

	return C.ulonglong(dirBlock)
}

//export vfsl_get_file
func vfsl_get_file(handle C.int, dirBlock C.ulonglong, fileName *C.char) C.ulonglong {
	vfs := getVFS(int(handle))
	if vfs == nil {
		return 0 // Invalid handle
	}

	goFileName := C.GoString(fileName)

	fileBlock, err := vfs.GetFile(uint64(dirBlock), goFileName)
	if err != nil {
		fmt.Printf("Error getting file: %v\n", err)
		return 0 // Error
	}

	return C.ulonglong(fileBlock)
}

//export vfsl_get_file_by_path
func vfsl_get_file_by_path(handle C.int, startDir C.ulonglong, path *C.char) C.ulonglong {
	vfs := getVFS(int(handle))
	if vfs == nil {
		return 0 // Invalid handle
	}

	goPath := C.GoString(path)

	fileBlock, err := vfs.GetFileByPath(uint64(startDir), goPath)
	if err != nil {
		fmt.Printf("Error getting file by path: %v\n", err)
		return 0 // Error
	}

	return C.ulonglong(fileBlock)
}

//export vfsl_delete_file
func vfsl_delete_file(handle C.int, fileBlock C.ulonglong) C.int {
	vfs := getVFS(int(handle))
	if vfs == nil {
		return C.int(-1) // Invalid handle
	}

	err := vfs.DeleteFile(uint64(fileBlock))
	if err != nil {
		fmt.Printf("Error deleting file: %v\n", err)
		return C.int(-2) // Error
	}

	return C.int(0) // Success
}

//export vfsl_delete_file_from_parent
func vfsl_delete_file_from_parent(handle C.int, parentDir C.ulonglong, fileName *C.char) C.int {
	vfs := getVFS(int(handle))
	if vfs == nil {
		return C.int(-1) // Invalid handle
	}

	goFileName := C.GoString(fileName)

	err := vfs.DeleteFileFromParent(uint64(parentDir), goFileName)
	if err != nil {
		fmt.Printf("Error deleting file from parent: %v\n", err)
		return C.int(-2) // Error
	}

	return C.int(0) // Success
}

//export vfsl_delete_directory
func vfsl_delete_directory(handle C.int, dirBlock C.ulonglong) C.int {
	vfs := getVFS(int(handle))
	if vfs == nil {
		return C.int(-1) // Invalid handle
	}

	err := vfs.DeleteDirectory(uint64(dirBlock))
	if err != nil {
		fmt.Printf("Error deleting directory: %v\n", err)
		return C.int(-2) // Error
	}

	return C.int(0) // Success
}

//export vfsl_delete_directory_from_parent
func vfsl_delete_directory_from_parent(handle C.int, parentDir C.ulonglong, dirName *C.char) C.int {
	vfs := getVFS(int(handle))
	if vfs == nil {
		return C.int(-1) // Invalid handle
	}

	goDirName := C.GoString(dirName)

	err := vfs.DeleteDirectoryFromParent(uint64(parentDir), goDirName)
	if err != nil {
		fmt.Printf("Error deleting directory from parent: %v\n", err)
		return C.int(-2) // Error
	}

	return C.int(0) // Success
}

//export vfsl_delete_by_path
func vfsl_delete_by_path(handle C.int, startDir C.ulonglong, path *C.char) C.int {
	vfs := getVFS(int(handle))
	if vfs == nil {
		return C.int(-1) // Invalid handle
	}

	goPath := C.GoString(path)

	err := vfs.DeleteByPath(uint64(startDir), goPath)
	if err != nil {
		fmt.Printf("Error deleting by path: %v\n", err)
		return C.int(-2) // Error
	}

	return C.int(0) // Success
}

//export vfsl_rename_file
func vfsl_rename_file(handle C.int, dirBlock C.ulonglong, oldName *C.char, newName *C.char) C.int {
	vfs := getVFS(int(handle))
	if vfs == nil {
		return C.int(-1) // Invalid handle
	}

	goOldName := C.GoString(oldName)
	goNewName := C.GoString(newName)

	err := vfs.RenameFile(uint64(dirBlock), goOldName, goNewName)
	if err != nil {
		fmt.Printf("Error renaming file: %v\n", err)
		return C.int(-2) // Error
	}

	return C.int(0) // Success
}

//export vfsl_rename_directory
func vfsl_rename_directory(handle C.int, parentDir C.ulonglong, oldName *C.char, newName *C.char) C.int {
	vfs := getVFS(int(handle))
	if vfs == nil {
		return C.int(-1) // Invalid handle
	}

	goOldName := C.GoString(oldName)
	goNewName := C.GoString(newName)

	err := vfs.RenameDirectory(uint64(parentDir), goOldName, goNewName)
	if err != nil {
		fmt.Printf("Error renaming directory: %v\n", err)
		return C.int(-2) // Error
	}

	return C.int(0) // Success
}

//export vfsl_rename_by_path
func vfsl_rename_by_path(handle C.int, startDir C.ulonglong, path *C.char, newName *C.char) C.int {
	vfs := getVFS(int(handle))
	if vfs == nil {
		return C.int(-1) // Invalid handle
	}

	goPath := C.GoString(path)
	goNewName := C.GoString(newName)

	err := vfs.RenameByPath(uint64(startDir), goPath, goNewName)
	if err != nil {
		fmt.Printf("Error renaming by path: %v\n", err)
		return C.int(-2) // Error
	}

	return C.int(0) // Success
}

//export vfsl_list_children
func vfsl_list_children(handle C.int, blockNum C.ulonglong, buffer unsafe.Pointer, bufferSize C.size_t) C.size_t {
	vfs := getVFS(int(handle))
	if vfs == nil {
		return 0 // Invalid handle
	}

	children, err := vfs.ListChildren(uint64(blockNum))
	if err != nil {
		fmt.Printf("Error listing children: %v\n", err)
		return 0 // Error
	}

	// Convert children to a JSON array for easier parsing in C
	childrenJSON, err := json.Marshal(children)
	if err != nil {
		fmt.Printf("Error marshaling children: %v\n", err)
		return 0 // Error
	}

	// Make sure we don't overflow the buffer
	copyLen := len(childrenJSON)
	if int(bufferSize) < copyLen {
		copyLen = int(bufferSize)
	}

	// Copy the data to the provided buffer
	dst := (*[1 << 30]byte)(buffer)[:copyLen]
	copy(dst, childrenJSON[:copyLen])

	return C.size_t(copyLen)
}

//export vfsl_get_block_type
func vfsl_get_block_type(handle C.int, blockNum C.ulonglong) C.int {
	vfs := getVFS(int(handle))
	if vfs == nil {
		return C.int(-1) // Invalid handle
	}

	blockType, err := vfs.GetBlockType(uint64(blockNum))
	if err != nil {
		fmt.Printf("Error getting block type: %v\n", err)
		return C.int(-2) // Error
	}

	return C.int(blockType)
}

//export vfsl_get_block_data
func vfsl_get_block_data(handle C.int, blockNum C.ulonglong, buffer unsafe.Pointer, bufferSize C.size_t) C.size_t {
	vfs := getVFS(int(handle))
	if vfs == nil {
		return 0 // Invalid handle
	}

	data, err := vfs.GetBlockData(uint64(blockNum))
	if err != nil {
		fmt.Printf("Error getting block data: %v\n", err)
		return 0 // Error
	}

	// Make sure we don't overflow the buffer
	copyLen := len(data)
	if int(bufferSize) < copyLen {
		copyLen = int(bufferSize)
	}

	// Copy the data to the provided buffer
	dst := (*[1 << 30]byte)(buffer)[:copyLen]
	copy(dst, data[:copyLen])

	return C.size_t(copyLen)
}

//export vfsl_get_block_metadata
func vfsl_get_block_metadata(handle C.int, blockNum C.ulonglong, buffer unsafe.Pointer, bufferSize C.size_t) C.size_t {
	vfs := getVFS(int(handle))
	if vfs == nil {
		return 0 // Invalid handle
	}

	meta, err := vfs.GetBlockMetadata(uint64(blockNum))
	if err != nil {
		fmt.Printf("Error getting block metadata: %v\n", err)
		return 0 // Error
	}

	// Convert metadata to JSON for easier parsing in C
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		fmt.Printf("Error marshaling metadata: %v\n", err)
		return 0 // Error
	}

	// Make sure we don't overflow the buffer
	copyLen := len(metaJSON)
	if int(bufferSize) < copyLen {
		copyLen = int(bufferSize)
	}

	// Copy the data to the provided buffer
	dst := (*[1 << 30]byte)(buffer)[:copyLen]
	copy(dst, metaJSON[:copyLen])

	return C.size_t(copyLen)
}

//export vfsl_set_extra_metadata
func vfsl_set_extra_metadata(handle C.int, blockNum C.ulonglong, key *C.char, value *C.char) C.int {
	vfs := getVFS(int(handle))
	if vfs == nil {
		return C.int(-1) // Invalid handle
	}

	goKey := C.GoString(key)
	goValue := C.GoString(value)

	err := vfs.SetExtraMetadata(uint64(blockNum), goKey, goValue)
	if err != nil {
		fmt.Printf("Error setting extra metadata: %v\n", err)
		return C.int(-2) // Error
	}

	return C.int(0) // Success
}

//export vfsl_get_extra_metadata
func vfsl_get_extra_metadata(handle C.int, blockNum C.ulonglong, key *C.char, buffer unsafe.Pointer, bufferSize C.size_t) C.int {
	vfs := getVFS(int(handle))
	if vfs == nil {
		return C.int(-1) // Invalid handle
	}

	goKey := C.GoString(key)

	value, exists, err := vfs.GetExtraMetadata(uint64(blockNum), goKey)
	if err != nil {
		fmt.Printf("Error getting extra metadata: %v\n", err)
		return C.int(-2) // Error
	}

	if !exists {
		return C.int(0) // Key not found
	}

	// Make sure we don't overflow the buffer
	copyLen := len(value)
	if int(bufferSize) < copyLen {
		copyLen = int(bufferSize)
	}

	// Copy the value to the provided buffer
	dst := (*[1 << 30]byte)(buffer)[:copyLen]
	copy(dst, []byte(value[:copyLen]))

	return C.int(1) // Key found
}

//export vfsl_create_stream_writer
func vfsl_create_stream_writer(handle C.int, parentBlock C.ulonglong, fileName *C.char) C.int {
	vfs := getVFS(int(handle))
	if vfs == nil {
		return C.int(-1) // Invalid handle
	}

	goFileName := C.GoString(fileName)

	writer, err := vfs.CreateStreamWriter(uint64(parentBlock), goFileName)
	if err != nil {
		fmt.Printf("Error creating stream writer: %v\n", err)
		return C.int(-2) // Error
	}

	writerId := storeWriter(writer)
	return C.int(writerId)
}

//export vfsl_stream_writer_write
func vfsl_stream_writer_write(writerHandle C.int, data unsafe.Pointer, dataSize C.size_t) C.int {
	writer := getWriter(int(writerHandle))
	if writer == nil {
		return C.int(-1) // Invalid handle
	}

	goData := C.GoBytes(data, C.int(dataSize))

	written, err := writer.Write(goData)
	if err != nil {
		fmt.Printf("Error writing to stream: %v\n", err)
		return C.int(-2) // Error
	}

	return C.int(written)
}

//export vfsl_stream_writer_close
func vfsl_stream_writer_close(writerHandle C.int) C.int {
	writer := getWriter(int(writerHandle))
	if writer == nil {
		return C.int(-1) // Invalid handle
	}

	err := writer.Close()
	removeWriter(int(writerHandle))

	if err != nil {
		fmt.Printf("Error closing stream writer: %v\n", err)
		return C.int(-2) // Error
	}

	return C.int(0) // Success
}

//export vfsl_open_stream_reader
func vfsl_open_stream_reader(handle C.int, fileBlock C.ulonglong) C.int {
	vfs := getVFS(int(handle))
	if vfs == nil {
		return C.int(-1) // Invalid handle
	}

	reader, err := vfs.OpenStreamReader(uint64(fileBlock))
	if err != nil {
		fmt.Printf("Error opening stream reader: %v\n", err)
		return C.int(-2) // Error
	}

	readerId := storeReader(reader)
	return C.int(readerId)
}

//export vfsl_stream_reader_read
func vfsl_stream_reader_read(readerHandle C.int, buffer unsafe.Pointer, bufferSize C.size_t) C.int {
	reader := getReader(int(readerHandle))
	if reader == nil {
		return C.int(-1) // Invalid handle
	}

	// Create a Go slice that references the C buffer
	goBuffer := make([]byte, int(bufferSize))

	read, err := reader.Read(goBuffer)
	if err != nil {
		if err.Error() == "EOF" {
			// Copy whatever we read before EOF
			if read > 0 {
				dst := (*[1 << 30]byte)(buffer)[:read]
				copy(dst, goBuffer[:read])
			}
			return C.int(-3) // EOF
		}

		fmt.Printf("Error reading from stream: %v\n", err)
		return C.int(-2) // Error
	}

	// Copy the data to the provided buffer
	dst := (*[1 << 30]byte)(buffer)[:read]
	copy(dst, goBuffer[:read])

	return C.int(read)
}

//export vfsl_stream_reader_close
func vfsl_stream_reader_close(readerHandle C.int) C.int {
	reader := getReader(int(readerHandle))
	if reader == nil {
		return C.int(-1) // Invalid handle
	}

	err := reader.Close()
	removeReader(int(readerHandle))

	if err != nil {
		fmt.Printf("Error closing stream reader: %v\n", err)
		return C.int(-2) // Error
	}

	return C.int(0) // Success
}

//export vfsl_add_child
func vfsl_add_child(handle C.int, parentBlock C.ulonglong, childBlock C.ulonglong) C.int {
	vfs := getVFS(int(handle))
	if vfs == nil {
		return C.int(-1) // Invalid handle
	}

	err := vfs.AddChild(uint64(parentBlock), uint64(childBlock))
	if err != nil {
		fmt.Printf("Error adding child: %v\n", err)
		return C.int(-2) // Error
	}

	return C.int(0) // Success
}

//export vfsl_remove_child
func vfsl_remove_child(handle C.int, parentBlock C.ulonglong, childBlock C.ulonglong) C.int {
	vfs := getVFS(int(handle))
	if vfs == nil {
		return C.int(-1) // Invalid handle
	}

	err := vfs.RemoveChild(uint64(parentBlock), uint64(childBlock))
	if err != nil {
		fmt.Printf("Error removing child: %v\n", err)
		return C.int(-2) // Error
	}

	return C.int(0) // Success
}

//export vfsl_update_file
func vfsl_update_file(handle C.int, fileBlock C.ulonglong, data unsafe.Pointer, dataSize C.size_t) C.int {
	vfs := getVFS(int(handle))
	if vfs == nil {
		return C.int(-1) // Invalid handle
	}

	goData := C.GoBytes(data, C.int(dataSize))

	err := vfs.UpdateFile(uint64(fileBlock), goData)
	if err != nil {
		fmt.Printf("Error updating file: %v\n", err)
		return C.int(-2) // Error
	}

	return C.int(0) // Success
}

//export vfsl_update_file_by_path
func vfsl_update_file_by_path(handle C.int, startDir C.ulonglong, path *C.char, data unsafe.Pointer, dataSize C.size_t) C.int {
	vfs := getVFS(int(handle))
	if vfs == nil {
		return C.int(-1) // Invalid handle
	}

	goPath := C.GoString(path)
	goData := C.GoBytes(data, C.int(dataSize))

	err := vfs.UpdateFileByPath(uint64(startDir), goPath, goData)
	if err != nil {
		fmt.Printf("Error updating file by path: %v\n", err)
		return C.int(-2) // Error
	}

	return C.int(0) // Success
}

//export vfsl_append_data
func vfsl_append_data(handle C.int, fileBlock C.ulonglong, data unsafe.Pointer, dataSize C.size_t) C.int {
	vfs := getVFS(int(handle))
	if vfs == nil {
		return C.int(-1) // Invalid handle
	}

	goData := C.GoBytes(data, C.int(dataSize))

	err := vfs.AppendData(uint64(fileBlock), goData)
	if err != nil {
		fmt.Printf("Error appending data: %v\n", err)
		return C.int(-2) // Error
	}

	return C.int(0) // Success
}

//export vfsl_get_block_header
func vfsl_get_block_header(handle C.int, blockNum C.ulonglong, buffer unsafe.Pointer, bufferSize C.size_t) C.size_t {
	vfs := getVFS(int(handle))
	if vfs == nil {
		return 0 // Invalid handle
	}

	header, _, _, _, err := vfs.ReadBlock(uint64(blockNum))
	if err != nil {
		fmt.Printf("Error reading block header: %v\n", err)
		return 0 // Error
	}

	// Convert header to JSON for easier parsing in C
	headerJSON, err := json.Marshal(header)
	if err != nil {
		fmt.Printf("Error marshaling header: %v\n", err)
		return 0 // Error
	}

	// Make sure we don't overflow the buffer
	copyLen := len(headerJSON)
	if int(bufferSize) < copyLen {
		copyLen = int(bufferSize)
	}

	// Copy the data to the provided buffer
	dst := (*[1 << 30]byte)(buffer)[:copyLen]
	copy(dst, headerJSON[:copyLen])

	return C.size_t(copyLen)
}

//export vfsl_update_block_metadata
func vfsl_update_block_metadata(handle C.int, blockNum C.ulonglong, metadataJSON *C.char) C.int {
	vfs := getVFS(int(handle))
	if vfs == nil {
		return C.int(-1) // Invalid handle
	}

	// Parse JSON metadata
	var updatedMeta vfslite.Metadata
	err := json.Unmarshal([]byte(C.GoString(metadataJSON)), &updatedMeta)
	if err != nil {
		fmt.Printf("Error unmarshaling metadata JSON: %v\n", err)
		return C.int(-2) // JSON parsing error
	}

	// Read existing block
	header, _, data, references, err := vfs.ReadBlock(uint64(blockNum))
	if err != nil {
		fmt.Printf("Error reading block: %v\n", err)
		return C.int(-3) // Block read error
	}

	// Write back with updated metadata
	err = vfs.WriteBlock(uint64(blockNum), header, &updatedMeta, data, references)
	if err != nil {
		fmt.Printf("Error writing block: %v\n", err)
		return C.int(-4) // Block write error
	}

	return C.int(0) // Success
}

//export vfsl_rename_block
func vfsl_rename_block(handle C.int, blockNum C.ulonglong, newName *C.char) C.int {
	vfs := getVFS(int(handle))
	if vfs == nil {
		return C.int(-1) // Invalid handle
	}

	goNewName := C.GoString(newName)

	err := vfs.RenameBlock(uint64(blockNum), goNewName)
	if err != nil {
		fmt.Printf("Error renaming block: %v\n", err)
		return C.int(-2) // Error
	}

	return C.int(0) // Success
}

// Required to build as a C shared library
func main() {}
