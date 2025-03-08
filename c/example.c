#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include "vfslite.h"

#define BUFFER_SIZE 4096

print_error(operation)
char *operation;
{
    fprintf(stderr, "Error during %s\n", operation);
}

main()
{
    int vfs;
    uint64_t root_block, dir_block, file_block, found_file_block, large_file_block;
    int writer, reader, meta_result;
    size_t read;
    char list_buffer[BUFFER_SIZE], read_buffer[BUFFER_SIZE], meta_buffer[BUFFER_SIZE];
    char buffer[1024], stream_buffer[1024];
    const char *file_content = "This is a test file for VFSLite C API.";

    vfs = vfsl_open("example.vfs", 4096, 100);
    if (vfs < 0) {
        print_error("opening VFSLite disk");
        return 1;
    }
    printf("VFSLite disk opened successfully with handle: %d\n", vfs);

    root_block = vfsl_get_root_block(vfs);
    if (root_block == 0) {
        print_error("getting root block");
        vfsl_close(vfs);
        return 1;
    }
    printf("Root block: %llu\n", (unsigned long long)root_block);

    dir_block = vfsl_create_directory_block(vfs, root_block, "example_dir");
    if (dir_block == 0) {
        print_error("creating directory");
        vfsl_close(vfs);
        return 1;
    }
    printf("Created directory 'example_dir' with block: %llu\n", (unsigned long long)dir_block);

    file_block = vfsl_write_file(vfs, dir_block, "test.txt", file_content, strlen(file_content));
    if (file_block == 0) {
        print_error("writing file");
        vfsl_close(vfs);
        return 1;
    }
    printf("Created file 'test.txt' with block: %llu\n", (unsigned long long)file_block);

    writer = vfsl_create_stream_writer(vfs, dir_block, "large.dat");
    if (writer < 0) {
        print_error("creating stream writer");
        vfsl_close(vfs);
        return 1;
    }

    for (int i = 0; i < 10; i++) {
        sprintf(buffer, "This is chunk %d of the large file.\n", i + 1);
        if (vfsl_stream_writer_write(writer, buffer, strlen(buffer)) < 0) {
            print_error("writing to stream");
            vfsl_stream_writer_close(writer);
            vfsl_close(vfs);
            return 1;
        }
        printf("Wrote %d bytes to stream\n", strlen(buffer));
    }

    if (vfsl_stream_writer_close(writer) < 0) {
        print_error("closing stream writer");
        vfsl_close(vfs);
        return 1;
    }
    printf("Large file created successfully\n");

    read = vfsl_list_children(vfs, dir_block, list_buffer, BUFFER_SIZE);
    if (read == 0) {
        print_error("listing children");
        vfsl_close(vfs);
        return 1;
    }
    list_buffer[read < BUFFER_SIZE ? read : BUFFER_SIZE - 1] = '\0';
    printf("Directory contents (JSON): %s\n", list_buffer);

    read = vfsl_read_file(vfs, file_block, read_buffer, BUFFER_SIZE);
    if (read == 0) {
        print_error("reading file");
        vfsl_close(vfs);
        return 1;
    }
    read_buffer[read < BUFFER_SIZE ? read : BUFFER_SIZE - 1] = '\0';
    printf("File content: %s\n", read_buffer);

    found_file_block = vfsl_get_file_by_path(vfs, root_block, "example_dir/test.txt");
    if (found_file_block == 0) {
        print_error("getting file by path");
        vfsl_close(vfs);
        return 1;
    }
    printf("Found file block by path: %llu\n", (unsigned long long)found_file_block);

    large_file_block = vfsl_get_file_by_path(vfs, root_block, "example_dir/large.dat");
    if (large_file_block == 0) {
        print_error("getting large file by path");
        vfsl_close(vfs);
        return 1;
    }

    reader = vfsl_open_stream_reader(vfs, large_file_block);
    if (reader < 0) {
        print_error("opening stream reader");
        vfsl_close(vfs);
        return 1;
    }

    printf("Large file content:\n");
    while (1) {
        int bytes_read = vfsl_stream_reader_read(reader, stream_buffer, sizeof(stream_buffer) - 1);
        if (bytes_read == VFSLITE_ERROR_EOF) break;
        if (bytes_read < 0) {
            print_error("reading from stream");
            vfsl_stream_reader_close(reader);
            vfsl_close(vfs);
            return 1;
        }
        stream_buffer[bytes_read] = '\0';
        printf("%s", stream_buffer);
    }

    if (vfsl_stream_reader_close(reader) < 0) {
        print_error("closing stream reader");
        vfsl_close(vfs);
        return 1;
    }

    if (vfsl_set_extra_metadata(vfs, file_block, "author", "VFSLite User") < 0) {
        print_error("setting metadata");
        vfsl_close(vfs);
        return 1;
    }

    meta_result = vfsl_get_extra_metadata(vfs, file_block, "author", meta_buffer, BUFFER_SIZE);
    if (meta_result < 0) {
        print_error("getting metadata");
        vfsl_close(vfs);
        return 1;
    }

    if (meta_result == 1) {
        meta_buffer[strlen(meta_buffer) < BUFFER_SIZE ? strlen(meta_buffer) : BUFFER_SIZE - 1] = '\0';
        printf("File metadata - author: %s\n", meta_buffer);
    } else {
        printf("Metadata key 'author' not found\n");
    }

    if (vfsl_rename_file(vfs, dir_block, "test.txt", "renamed.txt") < 0) {
        print_error("renaming file");
        vfsl_close(vfs);
        return 1;
    }
    printf("File renamed from 'test.txt' to 'renamed.txt'\n");

    if (vfsl_delete_file_from_parent(vfs, dir_block, "renamed.txt") < 0) {
        print_error("deleting file");
        vfsl_close(vfs);
        return 1;
    }
    printf("File 'renamed.txt' deleted\n");

    if (vfsl_close(vfs) < 0) {
        print_error("closing VFSLite disk");
        return 1;
    }
    printf("VFSLite disk closed successfully\n");
    return 0;
}