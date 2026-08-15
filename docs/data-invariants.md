# Data Invariants

This document records data-shape invariants observed in the Blizzard static API and the corresponding assumptions enforced by the scraper.

## Recipe Hierarchy Membership

Investigation performed against the EU static API on 2026-08-12.

Observed dataset:

- 26 professions
- 11,020 recipe hierarchy memberships
- 11,020 unique recipe IDs
- No recipe ID appeared under multiple profession, skill-tier, or category locations

### Invariant

A recipe ID belongs to exactly one profession, skill tier, and category tuple.

The scraper rejects duplicate recipe IDs discovered during hierarchy traversal rather than silently selecting the first location. A duplicate indicates either an upstream API contract change or that the data model must support many-to-many recipe membership.

## Recipe Output Shapes

Observed output shapes across all 11,020 recipe details:

| Shape | Count |
| --- | ---: |
| `crafted_item` only | 6,569 |
| Both `alliance_crafted_item` and `horde_crafted_item` | 587 |
| No crafted-item field | 3,864 |
| Only one faction-specific crafted item | 0 |
| Generic and faction-specific crafted items together | 0 |

### Invariants

The API currently exposes exactly three valid recipe output shapes:

1. **Generic output**
   - `crafted_item` is present.
   - Both faction-specific fields are absent.
   - Stored as one `Neutral` recipe variant.

2. **Faction-specific outputs**
   - Both `alliance_crafted_item` and `horde_crafted_item` are present.
   - `crafted_item` is absent.
   - Stored as separate `Alliance` and `Horde` recipe variants.

3. **No item output**
   - All three crafted-item fields are absent.
   - This is common for enchanting and other recipes whose effects are not represented as items.
   - Stored as one `Neutral` recipe variant with a `NULL` crafted item.

The scraper rejects these unobserved, ambiguous shapes:

- Only one faction-specific crafted item is present.
- A generic crafted item and either faction-specific crafted item are present together.

These errors should be investigated as upstream contract changes rather than handled through silent fallback behavior.

## Regional Static Data

Profession, skill-tier, category, recipe, reagent, and item metadata is treated as shared static data across regions. It is seeded once through a regional Blizzard client. Commodity auction data remains region-specific.

## Revalidation

These are observed API invariants, not guarantees published by Blizzard. Revalidate them when:

- Blizzard changes the static API schema.
- Duplicate hierarchy membership is reported.
- Recipe output-shape validation fails.
- Support for another game version or API namespace is introduced.
