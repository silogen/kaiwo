# Python utils

This folder contains the Python utils for Kaiwo. Please see the following for documentation on the available tools:

* [Kaiwo downloader](./docs/downloader.md), a tool to facilitate downloading data from various object storage backends and Hugging Face Hub 

## Getting started

### Prerequisites

- Python 3.8 or higher
- [pip](https://pip.pypa.io/)
- Required Python packages as listed in `requirements.txt`

### Installation

1. **Clone the repository:**

```bash
git clone https://github.com/silogen/kaiwo.git
cd kaiwo/python
```

2. **Install dependencies:**

```bash
pip install -r requirements.txt
```

## Downloader

The Kaiwo downloader is a Python tool designed to facilitate the download of files from various cloud sources. Its primary target is Kubernetes init containers, where files must be pre-fetched before application startup. The tool supports multiple cloud providers (Amazon S3, Google Cloud Storage, Azure Blob Storage, and Hugging Face Hub) and provides a unified, extensible configuration and execution framework.

* Object storage
  * S3-compatible storage endpoints
  * Azure Blob Storage
  * Google Cloud Storage
* Hugging Face Hub (complete repositories or individual files)

- [Features](#features)
- [Quick Start Guide](#quick-start-guide)
- [YAML Configuration](#yaml-configuration)
  - [Top-Level Configuration](#top-level-configuration)
  - [Download Task Configurations](#download-task-configurations)
  - [Union Types: CloudDownloadFolder vs. CloudDownloadFile](#union-types-clouddownloadfolder-vs-clouddownloadfile)
- [CLI Usage](#cli-usage)
- [Contributing](#contributing)
- [License](#license)

---

## Features

- **Multi-Provider Support:** Download files from Amazon S3, Google Cloud Storage, Azure Blob Storage, and Hugging Face Hub.
- **Extensible Handlers:** Easily add support for additional providers by implementing new handlers.
- **YAML-Based Configuration:** Use YAML files to describe complex download tasks including multiple buckets and items.
- **Concurrent Downloads:** Leverage multithreading for parallel file downloads with robust error handling and retries.
- **Kubernetes Friendly:** Optimized for use in Kubernetes init containers to pre-fetch files before container startup.
- **Detailed Logging:** Integrated with the [Rich](https://github.com/Textualize/rich) library for improved logging output.


### Configuration file

The Kaiwo downloader uses a YAML configuration file to define download tasks from various supported sources. The configuration file consists of the following key sections:

#### Basic structure

```yaml
download_root: <path_to_download_directory>
download_items:
  - type: <source_type>
    name: <task_name>
    <source_specific_configuration>
```

- **`download_root`**: The root directory where all downloaded files will be stored. **Note!** Hugging Face data does not obey this, but is downloaded in the HF cache folder.
- **`download_items`**: A list of download tasks, each specifying the source type and corresponding configuration.

#### **Supported Source Types and Configurations**

##### Hugging Face Hub

Downloads files or entire repositories from Hugging Face Hub.

```yaml
- type: hf
  name: Hugging Face download example
  repo_id: TinyLlama/TinyLlama-1.1B-Chat-v1.0
  files:
    - README.md
```

- **`repo_id`**: The repository ID on Hugging Face Hub.
- **`files`**: A list of files to download from the repository. If this is ommitted, the whole repository is downloaded.

##### Object storage endpoints

###### Google Cloud Storage (GCS)

Downloads files from GCS buckets.

```yaml
- type: gcs
  name: Test GCS
  application_credentials_path: <path_to_credentials>
  items:
    - bucket: test
      items:
        - path: .
          glob: "**/*"
          target_path: gcs/all
        - path: subdir
          glob: "**/*"
          target_path: gcs/sub
        - path: subdir/file02.txt
          target_path: gcs/single.txt
        - path: .
          glob: "**/*.md"
          target_path: gcs/glob
```

- **`project`**: GCP project ID.
- **`bucket`**: Name of the GCS bucket.
- **`items`**:
  - **`path`**: The path within the bucket to start downloading from.
  - **`glob`** (optional): Glob pattern to match specific files.
  - **`target_path`**: Local path relative to `download_root` where files will be stored.

---

#### **3. S3-Compatible Storage**

Downloads files from S3-compatible storage endpoints (e.g., MinIO).

```yaml
- type: s3
  name: Test Minio
  endpoint_url_file: test/secrets/s3/endpoint
  access_key_id_file: test/secrets/s3/access_key_id
  secret_key_file: test/secrets/s3/secret_key
  items:
    - bucket: test
      items:
        - path: .
          glob: "**/*"
          target_path: s3/all
        - path: subdir
          glob: "**/*"
          target_path: s3/sub
        - path: subdir/file02.txt
          target_path: s3/single.txt
```

- **`endpoint_url_file`**: Path to the file containing the S3 endpoint URL.
- **`access_key_id_file`**: Path to the file containing the S3 access key ID.
- **`secret_key_file`**: Path to the file containing the S3 secret key.
- **`bucket`**: Name of the S3 bucket.
- **`items`**: Same structure as GCS items.

---

#### **4. Azure Blob Storage**

Downloads files from Azure Blob Storage (or Azurite for testing).

```yaml
- type: azure-blob
  name: Test Azurite
  connection_string_file: test/secrets/azure/connection_string
  items:
    - container: test
      items:
        - path: .
          glob: "**/*"
          target_path: azure/all
        - path: subdir
          glob: "**/*"
          target_path: azure/sub
        - path: subdir/file02.txt
          target_path: azure/single.txt
```

- **`connection_string_file`**: Path to the file containing the Azure Blob Storage connection string.
- **`container`**: Name of the container in Azure Blob Storage.
- **`items`**: Same structure as GCS items.

### Testing

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

### Docker image

You can build a Docker image with the downloader by running:

```bash
cd <project_root>
docker build -t kaiwo-python -f .docker/kaiwo-python/Dockerfile python
```

## TODO

- [ ] Share parallel executor between tasks
- [ ] Enable HF_HUB_ENABLE_HF_TRANSFER option
