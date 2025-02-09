from typing import List, Literal, Union

from cloudpathlib import CloudPath, GSClient
from pydantic import BaseModel

from kaiwo.downloader.handlers.base import (
    CloudDownloadFile,
    CloudDownloadFolder,
    CloudDownloadTask,
    CloudDownloadTaskConfigBase, CloudDownloadBucket,
)


class GCSDownloadTaskConfig(CloudDownloadTaskConfigBase):
    type: Literal["gcs"] = "gcs"

    application_credentials_file: str = None
    project: str = None
    buckets: List[CloudDownloadBucket]

    def get_client(self) -> GSClient:
        return GSClient(
            application_credentials=self.application_credentials_file,
            project=self.project,
        )

    def get_items(self) -> List[CloudDownloadTask]:
        arr = []
        client = self.get_client()
        for bucket in self.buckets:
            container_root = CloudPath(f"gs://{bucket.name}", client=client)
            for item in bucket.items:
                arr.extend(item.get_download_tasks(container_root))
        return arr
