# API Response Documentation

## Commodities API

**Endpoint:** `GET /data/wow/auctions/commodities`

**Response Keys:** `_links`, `auctions`

### Auction Object Shape

```json
{
  "id": 1186233707,
  "item": {
    "id": 72096
  },
  "quantity": 22,
  "unit_price": 219500,
  "time_left": "SHORT"
}
```

**Fields:**
- `id`: Unique auction identifier (integer)
- `item.id`: The commodity item ID (integer)
- `quantity`: Number of items in this auction stack (integer)
- `unit_price`: Price per single unit in copper (integer)
- `time_left`: Duration remaining - `SHORT`, `MEDIUM`, `LONG`, or `VERY_LONG`

---

## Notes

- All prices are in **copper** (100 copper = 1 silver, 100 silver = 1 gold)
- Commodities are **stackable crafting materials** (herbs, ore, cloth, leather, etc.)
- The `auctions` array contains **region-wide** commodity auctions
- Response also includes `_links` key (self, commodities hrefs)
