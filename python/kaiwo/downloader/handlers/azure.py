from typing import List, Literal, Union

from cloudpathlib import AzureBlobClient, CloudPath
from pydantic import BaseModel

from kaiwo.downloader.handlers.base import (
    CloudDownloadFile,
    CloudDownloadFolder,
    CloudDownloadTask,
    CloudDownloadTaskConfigBase,
)
from kaiwo.downloader.utils import read_value_or_from_file


class AzureBlobFileDownloadContainer(BaseModel):
    container: str
    items: List[Union[CloudDownloadFolder, CloudDownloadFile]]


class AzureBlobDownloadTaskConfig(CloudDownloadTaskConfigBase):

    type: Literal["azure-blob"] = "azure-blob"

    connection_string: str = None
    connection_string_file: str = None

    items: List[AzureBlobFileDownloadContainer]

    def get_client(self) -> AzureBlobClient:
        connection_string = read_value_or_from_file(self.connection_string, self.connection_string_file)
        return AzureBlobClient(connection_string=connection_string)

    def get_items(self) -> List[CloudDownloadTask]:
        arr = []
        client = self.get_client()
        for container in self.items:
            container_root = CloudPath(f"az://{container.container}", client=client)
            for item in container.items:
                arr.extend(item.get_download_tasks(container_root))
        return arr
