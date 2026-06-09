import asyncio
import os

from dotenv import load_dotenv

# Local Development. Does not trigger if no .env
load_dotenv()


# Auth Credentials
CLIENT_ID = os.environ["CLIENT_ID"]
CLIENT_SECRET = os.environ["CLIENT_SECRET"]

# Region Constants
BLIZZARD_URL_REGIONS_INFO = {
    "eu": {"url": "eu.api.blizzard.com", "namespace": "dynamic-eu"},
    "us": {"url": "us.api.blizzard.com", "namespace": "dynamic-us"},
}
