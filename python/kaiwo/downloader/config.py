from typing import List, Union

from pydantic import BaseModel

from kaiwo.downloader.handlers.azure import AzureBlobDownloadTaskConfig
from kaiwo.downloader.handlers.gcs import GCSDownloadTaskConfig
from kaiwo.downloader.handlers.huggingface import HuggingFaceDownloadTaskConfig
from kaiwo.downloader.handlers.s3 import S3DownloadTaskConfig


class DownloadTaskConfig(BaseModel):
    download_items: List[
        Union[
            S3DownloadTaskConfig,
            GCSDownloadTaskConfig,
            AzureBlobDownloadTaskConfig,
            HuggingFaceDownloadTaskConfig,
        ]
    ]
    download_root: str = None
