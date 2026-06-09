# Std lib
import time

# External
import httpx

# Project Imports
from src.api_response_types import (
    CommodityAuction,
    CommodityAuctionsApiResponse,
    OAuthTokenResponse,
)


# Urls & Endpoints
OAUTH_TOKEN_URL = "https://oauth.battle.net/token"
BLIZZARD_ENDPOINTS = {"commodities": "/data/wow/auctions/commodities"}


async def get_token(
    client: httpx.AsyncClient, client_id: str, client_secret: str
) -> tuple[str, float]:
    response = await client.post(
        url=OAUTH_TOKEN_URL,
        data={"grant_type": "client_credentials"},
        auth=(client_id, client_secret),
    )
    response.raise_for_status()
    data: OAuthTokenResponse = response.json()
    return data["access_token"], time.time() + data["expires_in"]


async def get_commodities(
    client: httpx.AsyncClient,
    region_url: str,
    endpoint: str,
    namespace: str,
    token: str,
) -> list[CommodityAuction]:
    url = f"https://{region_url}{endpoint}"
    headers = {"Authorization": f"Bearer {token}"}
    params = {"namespace": namespace}

    response = await client.get(url=url, headers=headers, params=params)
    response.raise_for_status()
    data: CommodityAuctionsApiResponse = response.json()
    return data["auctions"]
