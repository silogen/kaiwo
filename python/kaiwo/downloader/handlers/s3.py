from typing import List, Literal, Optional

from cloudpathlib import CloudPath, S3Client
from pydantic import Field

from kaiwo.downloader.handlers.base import (
    CloudDownloadBucket,
    CloudDownloadTask,
    CloudDownloadTaskConfigBase,
    ValueReference,
)


class S3DownloadTaskConfig(CloudDownloadTaskConfigBase):
    type: Literal["s3"] = "s3"

    endpoint_url: str = Field(alias="endpointUrl")
    access_key_id: Optional[ValueReference] = Field(default=None, alias="accessKeyId")
    secret_key: Optional[ValueReference] = Field(default=None, alias="secretKey")

    buckets: List[CloudDownloadBucket]

    def get_client(self) -> S3Client:
        endpoint_url = self.endpoint_url
        access_key_id = self.access_key_id if self.access_key_id is None else self.access_key_id.get_value()
        secret_access_key = self.secret_key if self.secret_key is None else self.secret_key.get_value()

        is_public = access_key_id is None and secret_access_key is None

        return S3Client(
            endpoint_url=endpoint_url,
            aws_access_key_id=access_key_id,
            aws_secret_access_key=secret_access_key,
            no_sign_request=is_public,
        )

    def get_items(self) -> List[CloudDownloadTask]:
        arr = []
        client = self.get_client()
        for bucket in self.buckets:
            container_root = CloudPath(f"s3://{bucket.name}", client=client)
            for item in bucket.items:
                arr.extend(item.get_download_tasks(container_root))
        return arr
