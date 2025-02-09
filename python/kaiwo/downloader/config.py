from typing import List, Union

from kaiwo.downloader.handlers.base import CloudDownloadTaskConfigBase
from pydantic import BaseModel, Field

from kaiwo.downloader.handlers.azure import AzureBlobDownloadTaskConfig
from kaiwo.downloader.handlers.gcs import GCSDownloadTaskConfig
from kaiwo.downloader.handlers.huggingface import HuggingFaceDownloadTaskConfig
from kaiwo.downloader.handlers.s3 import S3DownloadTaskConfig


class DownloadTaskConfig(BaseModel):
    s3: List[S3DownloadTaskConfig] = Field(default_factory=list)
    gcs: List[GCSDownloadTaskConfig] = Field(default_factory=list)
    azure_blob: List[AzureBlobDownloadTaskConfig] = Field(default_factory=list)
    hf: List[HuggingFaceDownloadTaskConfig] = Field(default_factory=list)
    download_root: str = None

    @property
    def download_items(self) -> List[CloudDownloadTaskConfigBase]:
        return self.s3 + self.gcs + self.azure_blob + self.hf
