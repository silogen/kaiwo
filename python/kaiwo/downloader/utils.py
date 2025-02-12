import logging
from abc import ABC, abstractmethod
from concurrent.futures import ThreadPoolExecutor, as_completed
from pathlib import Path
from typing import List

from cloudpathlib import CloudPath
from tenacity import retry, stop_after_attempt, wait_exponential

logger = logging.getLogger(__name__)


class DownloadTask(ABC):

    @abstractmethod
    def run(self) -> None:
        pass

    @property
    @abstractmethod
    def title(self) -> str:
        pass


@retry(stop=stop_after_attempt(3), wait=wait_exponential(multiplier=1, min=1, max=10))
def download_file(source: CloudPath, destination: Path) -> None:
    destination.parent.mkdir(exist_ok=True, parents=True)
    logger.info(f"Downloading {source} to {destination}")
    source.download_to(destination)


def parallel_downloads(tasks: List[DownloadTask], max_workers: int = 5) -> None:
    with ThreadPoolExecutor(max_workers=max_workers) as executor:
        # Submit all tasks to the executor
        future_to_task = {executor.submit(task.run): task for task in tasks}

        try:
            # Process tasks as they complete
            for future in as_completed(future_to_task):
                task = future_to_task[future]
                try:
                    future.result()  # Raises exception if occurred
                    logger.info(f"Completed download task: {task.title}")
                except Exception as e:
                    logger.error(f"Task failed: {task.title} with error: {e}")

                    # Cancel all other tasks if one fails
                    for fut in future_to_task:
                        if not fut.done():
                            fut.cancel()
                            logger.debug(f"Cancelled task: {future_to_task[fut].title}")

                    # Shutdown executor immediately
                    executor.shutdown(wait=False, cancel_futures=True)
                    raise  # Re-raise exception to propagate error

        except Exception as e:
            logger.critical(f"Download process aborted due to error: {e}")
            raise e  # Exit with failure
