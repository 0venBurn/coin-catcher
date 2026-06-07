import os

from dotenv import load_dotenv

# Local Development. Does not trigger if no .env
load_dotenv()

# Blizzard API Credentials
CLIENT_ID = os.environ["CLIENT_ID"]
CLIENT_SECRET = os.environ["CLIENT_SECRET"]

# Urls & Endpoints
OAUTH_TOKEN_URL = "https://oauth.battle.net/token"
BLIZZARD_ENDPOINTS = {"commodities": "/data/wow/auctions/commodities"}

# Region Constants
BLIZZARD_URL_REGIONS_INFO = {
    "eu": {"url": "eu.api.blizzard.com", "namespace": "dynamic-eu"},
    "us": {"url": "us.api.blizzard.com", "namespace": "dynamic-us"},
}
