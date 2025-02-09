import logging
from typing import List, Literal

from huggingface_hub import hf_hub_download, snapshot_download
from pydantic import BaseModel, Field

from kaiwo.downloader.handlers.base import DownloadTaskConfigBase
from kaiwo.downloader.utils import DownloadTask, parallel_downloads

logger = logging.getLogger(__name__)


class HuggingFaceDownloadTaskConfig(DownloadTaskConfigBase):
    type: Literal["hf"] = "hf"

    repo_id: str
    files: List[str] = Field(default_factory=list)

    def run(self, download_root: str = None, max_workers: int = 5) -> None:
        logger.info(f"Downloading from Hugging Face Hub repo '{self.repo_id}'")
        if download_root is not None:
            logger.warning(
                "Please note that the Hugging Face downloader does not respect any download root values, "
                "all downloads are performed into the Hugging Face local cache"
            )
        if not self.files:
            logger.info("Downloading all files from the repo")
            snapshot_download(repo_id=self.repo_id, max_workers=max_workers)
        else:
            tasks = []
            logger.info("Downloading specific files from the repo")
            for file in self.files:
                tasks.append(HuggingFaceDownloadTask(repo_id=self.repo_id, file=file))

            parallel_downloads(tasks, max_workers=max_workers)


class HuggingFaceDownloadTask(DownloadTask, BaseModel):
    repo_id: str
    file: str

    def run(self) -> None:
        logger.info(f"Downloading {self.title}")
        hf_hub_download(repo_id=self.repo_id, filename=self.file)

    @property
    def title(self) -> str:
        return f"huggingface.co/{self.repo_id}/{self.file}"
