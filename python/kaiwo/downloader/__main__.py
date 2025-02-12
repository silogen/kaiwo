import argparse
import logging

import yaml
from pydantic import ValidationError
from rich.logging import RichHandler

from kaiwo.downloader.config import DownloadTaskConfig

logger = logging.getLogger(__name__)


def load_config_from_yaml(file_path: str) -> DownloadTaskConfig:
    with open(file_path, "r") as f:
        data = yaml.safe_load(f)
    try:
        return DownloadTaskConfig.model_validate(data)
    except ValidationError as e:
        logger.error(f"Invalid configuration file: {e}")
        exit(1)


def main():
    parser = argparse.ArgumentParser(description="Cloud Downloader CLI")
    parser.add_argument("config", help="Path to the YAML configuration file")
    parser.add_argument("--max-workers", type=int, default=8, required=False)
    parser.add_argument(
        "--log-level",
        default="INFO",
        help="Set the logging level (DEBUG, INFO, WARNING, ERROR)",
    )

    args = parser.parse_args()

    logging.basicConfig(
        level=getattr(logging, args.log_level.upper(), logging.INFO),
        format="%(message)s",
        datefmt="[%X]",
        handlers=[
            RichHandler(
                omit_repeated_times=False,
                show_path=False,
                markup=True,
            )
        ],
    )

    # Silence verbose Azure logging

    azure_logger = logging.getLogger("azure.core.pipeline.policies.http_logging_policy")
    azure_logger.setLevel(logging.WARNING)

    if args.max_workers < 1:
        logger.error(f"Invalid value for max workers: {args.max_workers}")
        exit(1)

    try:
        config = load_config_from_yaml(args.config)
        for download_item in config.download_items:
            logger.info(f"Running download task: [bold]{download_item.title}[/]")
            download_item.run(download_root=config.download_root)
    except Exception as e:
        logger.error(f"Error occurred during downloads: {e}")
        exit(1)


if __name__ == "__main__":
    main()
