from typing import List, Literal

from cloudpathlib import AzureBlobClient, CloudPath
from pydantic import Field

from kaiwo.downloader.handlers.base import (
    CloudDownloadBucket,
    CloudDownloadTask,
    CloudDownloadTaskConfigBase,
    ValueReference,
)


class AzureBlobDownloadTaskConfig(CloudDownloadTaskConfigBase):

    type: Literal["azure-blob"] = "azure-blob"

    connection_string: ValueReference = Field(alias="connectionString")

    containers: List[CloudDownloadBucket]

    def get_client(self) -> AzureBlobClient:
        connection_string = self.connection_string.get_value()
        return AzureBlobClient(connection_string=connection_string)

    def get_items(self) -> List[CloudDownloadTask]:
        arr = []
        client = self.get_client()
        for container in self.containers:
            container_root = CloudPath(f"az://{container.name}", client=client)
            for item in container.items:
                arr.extend(item.get_download_tasks(container_root))
        return arr
