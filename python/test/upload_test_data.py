from google.cloud.exceptions import NotFound
from cloudpathlib import CloudPath, GSClient, S3Client, AzureBlobClient, AnyPath

import os
from google.cloud import storage
import boto3
from botocore.exceptions import ClientError
from azure.storage.blob import BlobServiceClient

TEST_BUCKET_NAME = "test"


def get_gcp_test_root() -> CloudPath:
    # Set the environment variable for the emulator

    os.environ["STORAGE_EMULATOR_HOST"] = "http://localhost:9023"
    client = storage.Client(project="test-project")

    bucket_name = TEST_BUCKET_NAME

    # Ensure bucket exists
    try:
        client.get_bucket(bucket_name)
    except NotFound:
        client.create_bucket(bucket_name)

    return AnyPath(f"gs://{bucket_name}", client=GSClient(storage_client=client))


def get_s3_test_root() -> CloudPath:
    s3_client = boto3.client(
        "s3",
        aws_access_key_id="minio",
        aws_secret_access_key="minio123",
        endpoint_url="http://localhost:9000",
    )

    bucket_name = TEST_BUCKET_NAME

    # Ensure bucket exists
    try:
        s3_client.head_bucket(Bucket=bucket_name)
    except ClientError:
        s3_client.create_bucket(Bucket=bucket_name)

    return AnyPath(
        f"s3://{bucket_name}",
        client=S3Client(
            aws_access_key_id="minio",
            aws_secret_access_key="minio123",
            endpoint_url="http://localhost:9000",
        ),
    )


def get_azure_test_root() -> CloudPath:
    container_name = TEST_BUCKET_NAME
    connection_string = "DefaultEndpointsProtocol=http;AccountName=devstoreaccount1;AccountKey=Eby8vdM02xNOcqFlqUwJPLlmEtlCDXJ1OUzFT50uSRZ6IFsuFq2UVErCz4I6tq/K1SZFPTOtr/KBHBeksoGMGw==;BlobEndpoint=http://127.0.0.1:10000/devstoreaccount1;QueueEndpoint=http://127.0.0.1:10001/devstoreaccount1;TableEndpoint=http://127.0.0.1:10002/devstoreaccount1;"

    blob_service_client = BlobServiceClient.from_connection_string(connection_string)
    client = AzureBlobClient(blob_service_client=blob_service_client)

    # Ensure container exists
    container_client = blob_service_client.get_container_client(container_name)
    try:
        container_client.get_container_properties()
    except Exception:
        blob_service_client.create_container(container_name)

    return AnyPath(f"az://{container_name}", client=client)


def upload_test_data(root: CloudPath) -> None:
    root.upload_from("data", force_overwrite_to_cloud=True)


if __name__ == "__main__":
    print("Uploading GCP test data to local emulator")
    upload_test_data(get_gcp_test_root())
    print("Uploading S3 test data to local Minio")
    upload_test_data(get_s3_test_root())
    print("Uploading Azure test data to local azurite")
    upload_test_data(get_azure_test_root())
    print("Done!")
