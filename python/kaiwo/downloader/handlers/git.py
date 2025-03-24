import logging
import pathlib
import shutil
import tempfile
from typing import List, Literal

import git
from cloudpathlib import AzureBlobClient, CloudPath
from pydantic import Field

from kaiwo.downloader.handlers.base import (
    CloudDownloadTask,
    DownloadTaskConfigBase,
    ValueReference,
)

logger = logging.getLogger(__name__)


class GitDownloadTaskConfig(DownloadTaskConfigBase):

    type: Literal["git"] = "git"

    repository: str
    branch: str = None
    commit: str = None
    username: ValueReference = None
    token: ValueReference = None

    path: str = None
    target_path: str = Field(alias="targetPath")

    def run(self, download_root: str = None, max_workers: int = 5) -> None:
        logger.info(f"Downloading from repository: {self.repository}")

        with tempfile.TemporaryDirectory() as temp_dir:
            auth = ""
            if self.token is not None:
                logger.debug("Using authentication token")
                auth = f"{self.token.get_value()}@"
            if self.username is not None:
                logger.debug("Using username")
                auth = f"{self.username.get_value()}:" + auth

            auth_url = self.repository.replace("https://", f"https://{auth}")
            repo = git.Repo.clone_from(auth_url, temp_dir, depth=1)

            if self.commit is not None:
                logger.info(f"Using commit: {self.commit}")
                repo.git.fetch("origin", self.commit, depth=1)
                repo.git.checkout(self.commit)
            elif self.branch is not None:
                logger.info(f"Using branch: {self.branch}")
                repo.git.fetch("origin", self.branch, depth=1)
                repo.git.checkout(self.branch)

            path = self.path if self.path is not None else "."

            source = pathlib.Path(temp_dir) / path
            if download_root is None:
                destination = pathlib.Path(".") / self.target_path
            else:
                destination = pathlib.Path(download_root) / self.target_path

            logger.info(f"Copying from {path} to {destination}")

            if source.is_file():
                destination.parent.mkdir(parents=True, exist_ok=True)  # Ensure the parent folder exists
                shutil.copy(source, destination)
            else:
                destination.mkdir(parents=True, exist_ok=True)  # Ensure the target directory exists
                shutil.copytree(source, destination, dirs_exist_ok=True)

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
