import asyncio

from src.blizzard_api_client import main as run_scraper


def main() -> None:
    asyncio.run(run_scraper())
