from typing import List

from pydantic import BaseModel, Field

from kaiwo.downloader.handlers.azure import AzureBlobDownloadTaskConfig
from kaiwo.downloader.handlers.base import CloudDownloadTaskConfigBase
from kaiwo.downloader.handlers.gcs import GCSDownloadTaskConfig
from kaiwo.downloader.handlers.git import GitDownloadTaskConfig
from kaiwo.downloader.handlers.huggingface import HuggingFaceDownloadTaskConfig
from kaiwo.downloader.handlers.s3 import S3DownloadTaskConfig


class DownloadTaskConfig(BaseModel):
    git: List[GitDownloadTaskConfig] = Field(default_factory=list)
    s3: List[S3DownloadTaskConfig] = Field(default_factory=list)
    gcs: List[GCSDownloadTaskConfig] = Field(default_factory=list)
    azure_blob: List[AzureBlobDownloadTaskConfig] = Field(default_factory=list, alias="azureBlob")
    hf: List[HuggingFaceDownloadTaskConfig] = Field(default_factory=list)
    download_root: str = Field(default=None, alias="downloadRoot")

    @property
    def download_items(self) -> List[CloudDownloadTaskConfigBase]:
        return self.s3 + self.gcs + self.azure_blob + self.hf + self.git
