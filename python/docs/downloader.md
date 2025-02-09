# Cloud Downloader

The Kaiwo Downloader is a Python tool designed to facilitate the download of files from various cloud sources. Its primary target is Kubernetes init containers, where files must be pre-fetched before application startup. The tool supports multiple cloud providers (Amazon S3, Google Cloud Storage, Azure Blob Storage, and Hugging Face Hub) and provides a unified, extensible configuration and execution framework.

## Table of Contents

- [Features](#features)
- [Quick Start Guide](#quick-start-guide)
- [YAML Configuration](#yaml-configuration)
  - [Top-Level Configuration](#top-level-configuration)
  - [Download Task Configurations](#download-task-configurations)
  - [Union Types: CloudDownloadFolder vs. CloudDownloadFile](#downloading-files-and-folders-from-object-storage)
- [CLI Usage](#cli-usage)
- [Testing](#testing)
- [Docker image](#docker-image)

## Features

- **Multi-Provider Support:** Download files from Amazon S3, Google Cloud Storage, Azure Blob Storage, and Hugging Face Hub.
- **Extensible Handlers:** Easily add support for additional providers by implementing new handlers.
- **YAML-Based Configuration:** Use YAML files to describe complex download tasks including multiple buckets and items.
- **Kubernetes-friendly:** Reference secrets as files mounted to disk to avoid having to include secrets directly into config
- **Concurrent Downloads:** Leverage multithreading for parallel file downloads with robust error handling and retries.
- **Detailed Logging:** Integrated with the [Rich](https://github.com/Textualize/rich) library for improved logging output.

## Quick Start Guide

### Running the Downloader

Prepare your YAML configuration file (see YAML Configuration below for details).

Run the downloader CLI:

```bash
python -m kaiwo.downloader path/to/your_config.yaml
```

This command reads your configuration file, validates it, and initiates the download tasks defined within it.

## YAML Configuration

The configuration file is written in YAML and defines the download tasks to execute. It comprises a top-level configuration with a list of download tasks and an optional download root. If the download root is not specified, the current working directory becomes the download root, unless the target path is an absolute path.

### Top-Level Configuration

```yaml
download_root: "/path/to/local/download"
download_items:
  - type: s3
    name: "MyS3Download"
    endpoint_url: "https://s3.amazonaws.com"
    access_key_id: "YOUR_ACCESS_KEY_ID"
    secret_key: "YOUR_SECRET_KEY"
    buckets:
      - name: "my-s3-bucket"
        items:
          - folder: "data"
            target_path: "local/data"
            glob: "**/*.csv"
          - file: "readme.txt"
            target_path: "local/readme.txt"
  - type: gcs
    name: "Google Cloud Storage"
    application_credentials_file: "/path/to/file"
    buckets:
      - name: "my-gcs-bucket"
        items:
          - file: "readme.txt"
            target_path: "local/readme.txt"
  - type: azure-blob
    connection_string: "connection_string"
    containers:
      - name: "my-azure-blob-container"
        items:
          - file: "readme.txt"
            target_path: "local/readme.txt"
  - type: hf
    name: "MyHFDownload"
    repo_id: "username/repo"
    files:
      - "model.bin"
      - "config.json"
```

### Download Task Configurations

Each download task is specified as an item in `download_items` with a type field determining which handler is used. Each task
shares the same base fields:

- `type` is the type of the field
- `name` is a descriptive name for the task (used for logging)

#### Object storage

##### S3

The following fields are specific to S3-compatible APIs 

- `type: s3`
- `endpoint_url`: The S3-API endpoint, or
- `endpoint_url_file`: A file that contains the S3-API endpoint
- `access_key_id`: The access key ID for authentication, or
- `access_key_id_file`: A file that contains the access key ID for authentication
- `secret_key`: The secret key for authentication, or
- `secret_key_file`: A file that contains the secret key for authentication

##### Google Cloud Storage (GCS)

The following fields are specific to GCS

- `type: gcs`
- `application_credentials_file`: The path to a JSON file containing the Google Cloud service account credentials
- `project`: (optional, used mainly for testing) the Google Cloud project ID associated with your GCS resources

##### Azure Blob Storage

- `type: gcs`
- `connection_string`: The connection string for Azure Blob Storage
- `connection_string_file`: A file path to a file containing the connection string

#### Hugging Face hub

- `type: hf`
- `repo_id`: The identifier for the Hugging Face repository in the format `owner/repo`
- `files`: A list of specific file names to download from the repository
  - If the list is empty, the downloader will invoke a snapshot download of the entire repository (which downloads into the Hugging Face cache)
  - When specific files are listed, only those files are downloaded

When using Kubernetes, use the `_file` fields to reference secrets mounted into the container rather than directly embedding the value into any config map.

### Downloading files and folders from object storage

For cloud providers that support both (sub)folder and file downloads (e.g., S3, GCS, Azure Blob), download items are specified within a bucket or container using either a folder entry:

```yaml
folder:
  folder: "remote_folder_path"
  target_path: "local_target_folder"
  glob: "**/*"  # Optional glob pattern (default is "**/*")
```

in which all the files under `remote_folder_path` in the given bucket or container are downloaded into `local_target_folder`. The alternative is a file entry:

```yaml
file:
  file: "remote_file_path"
  target_path: "local_target_file"
```

Only one of the two (folder or file) should be specified per item.

## CLI Usage

The main CLI is invoked as a module and accepts several parameters:

```bash
python -m kaiwo.downloader CONFIG_FILE [--max-workers MAX_WORKERS] [--log-level LOG_LEVEL]
```

* `CONFIG_FILE`: Path to the YAML configuration file.
* `--max-workers`: (Optional) Number of concurrent worker threads. Default is 8.
* `--log-level`: (Optional) Logging level. Accepts the levels DEBUG, INFO, WARNING, and ERROR. Default is INFO.

## Testing

For local testing, you can use the included storage endpoint emulators. These can be started by running:

```bash
cd <project_root>/python
docker compose -f test/storage.docker-compose.yaml up -d 
```

Next you can populate them with some sample data:

```bash
cd <project_root>/python/test
python upload_test_data.py
```

And finally you can run the download task:

```bash
cd <repository_root>/python
STORAGE_EMULATOR_HOST=http://localhost:9023 python -m kaiwo.downloader test/test_download_task.yaml 
```

This will download the objects listed in the download config to `<project_root>/python/test/output`.

## Docker image

You can build a Docker image with the downloader by running:

```bash
cd <project_root>
docker build -t kaiwo-downloader -f .docker/kaiwo-python/Dockerfile python
```

## TODO

- [ ] Share parallel executor between tasks
- [ ] Enable HF_HUB_ENABLE_HF_TRANSFER option
