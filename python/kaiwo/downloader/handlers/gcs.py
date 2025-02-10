from typing import List, Literal

from cloudpathlib import CloudPath, GSClient

from kaiwo.downloader.handlers.base import (
    CloudDownloadBucket,
    CloudDownloadTask,
    CloudDownloadTaskConfigBase, ValueReference,
)


class GCSDownloadTaskConfig(CloudDownloadTaskConfigBase):
    type: Literal["gcs"] = "gcs"

    application_credentials: ValueReference = None
    project: str = None
    buckets: List[CloudDownloadBucket]

    def get_client(self) -> GSClient:
        return GSClient(
            application_credentials=self.application_credentials.get_value() if self.application_credentials is not None else None,
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
