from typing import List, Literal, Union

from cloudpathlib import CloudPath, S3Client
from pydantic import BaseModel

from kaiwo.downloader.handlers.base import (
    CloudDownloadFile,
    CloudDownloadFolder,
    CloudDownloadTask,
    CloudDownloadTaskConfigBase,
)
from kaiwo.downloader.utils import read_value_or_from_file


class S3FileDownloadBucket(BaseModel):
    bucket: str
    items: List[Union[CloudDownloadFolder, CloudDownloadFile]]


class S3DownloadTaskConfig(CloudDownloadTaskConfigBase):
    type: Literal["s3"] = "s3"

    endpoint_url: str = None
    endpoint_url_file: str = None

    access_key_id: str = None
    access_key_id_file: str = None

    secret_key: str = None
    secret_key_file: str = None

    items: List[S3FileDownloadBucket]

    def get_client(self) -> S3Client:
        endpoint_url = read_value_or_from_file(self.endpoint_url, self.endpoint_url_file)
        access_key_id = read_value_or_from_file(self.access_key_id, self.access_key_id_file)
        secret_access_key = read_value_or_from_file(self.secret_key, self.secret_key_file)

        is_public = access_key_id is None and secret_access_key is None

        return S3Client(
            aws_access_key_id=access_key_id,
            aws_secret_access_key=secret_access_key,
            endpoint_url=endpoint_url,
            no_sign_request=is_public,
        )

    def get_items(self) -> List[CloudDownloadTask]:
        arr = []
        client = self.get_client()
        for bucket in self.items:
            container_root = CloudPath(f"s3://{bucket.bucket}", client=client)
            for item in bucket.items:
                arr.extend(item.get_download_tasks(container_root))
        return arr
