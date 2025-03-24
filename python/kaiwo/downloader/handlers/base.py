import logging
from abc import ABC, abstractmethod
from pathlib import Path
from typing import List

from cloudpathlib import CloudPath
from pydantic import BaseModel, Field

from kaiwo.downloader.utils import DownloadTask, download_file, parallel_downloads

logger = logging.getLogger(__name__)


class CloudDownloadTask(DownloadTask, BaseModel):
    cloud_path: CloudPath
    target_path: Path

    def run(self) -> None:
        logger.info(f"Downloading {self.title}")
        download_file(source=self.cloud_path, destination=self.target_path)

    @property
    def title(self) -> str:
        return f"{self.cloud_path} -> {self.target_path}"


class DownloadTaskConfigBase(BaseModel, ABC):
    type: str
    name: str = None

    @abstractmethod
    def run(self) -> None:
        pass

    @property
    def title(self) -> str:
        if self.name is None:
            return self.type.upper()
        return f"{self.name} ({self.type.upper()})"


class CloudDownloadTaskConfigBase(DownloadTaskConfigBase, ABC):

    def run(self, download_root: str = None, max_workers: int = 5) -> None:
        tasks = self.get_items()
        if download_root is not None:
            for task in tasks:
                task.target_path = Path(download_root) / task.target_path

        parallel_downloads(tasks, max_workers=max_workers)

    @abstractmethod
    def get_items(self) -> List[CloudDownloadTask]:
        pass


class CloudDownloadSource(BaseModel):

    @abstractmethod
    def get_download_tasks(self, root: CloudPath) -> List[CloudDownloadTask]:
        pass


class CloudDownloadFolder(CloudDownloadSource):
    path: str
    target_path: str = Field(alias="targetPath")
    glob: str = "**/*"

    def get_download_tasks(self, root: CloudPath) -> List[CloudDownloadTask]:
        tasks = []
        target_root = Path(self.target_path)
        cloud_folder = root / self.path
        logger.debug(f"Discovering files from {cloud_folder} ({self.glob})")
        for cloud_item in cloud_folder.glob(self.glob):
            tasks.append(
                CloudDownloadTask(
                    cloud_path=cloud_item,
                    target_path=target_root / cloud_item.relative_to(cloud_folder),
                )
            )
        return tasks


class CloudDownloadFile(CloudDownloadSource):
    path: str
    target_path: str = Field(alias="targetPath")

    def get_download_tasks(self, root: CloudPath) -> List[CloudDownloadTask]:
        return [
            CloudDownloadTask(
                cloud_path=root / self.path,
                target_path=Path(self.target_path),
            )
        ]


class CloudDownloadBucket(BaseModel):
    name: str
    # items: List[Union[CloudDownloadFolder, CloudDownloadFile]]
    files: List[CloudDownloadFile] = Field(default_factory=list)
    folders: List[CloudDownloadFolder] = Field(default_factory=list)

    @property
    def items(self) -> List[CloudDownloadSource]:
        return self.folders + self.files


class ValueReference(BaseModel):
    value: str = None
    file: str = None

    def get_value(self) -> str:
        if self.value is not None:
            return self.value
        if self.file is None:
            raise ValueError("Must provide either value or file")
        with open(self.file) as f:
            return f.read().strip()
