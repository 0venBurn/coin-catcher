from typing import TypedDict


# OAuth Token Response
class OAuthTokenResponse(TypedDict):
    access_token: str
    token_type: str
    expires_in: int
    scope: str


# Commodities
class CommodityItem(TypedDict):
    id: int


class CommodityAuction(TypedDict):
    id: int
    item: CommodityItem
    quantity: int
    unit_price: int
    time_left: str


class CommodityAuctionsApiResponse(TypedDict):
    auctions: list[CommodityAuction]
