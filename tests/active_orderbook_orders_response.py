from decimal import Decimal
from typing import List
import time
from datetime import datetime


class LimitOrderAsset:
    def __init__(self, symbol: str):
        self.symbol = symbol

    def validate(self):
        # Ensure symbol is not empty
        assert self.symbol, "Symbol must not be empty"

class LimitOrder:
    def __init__(self, tick_id: int, order_id: int, order_direction: str, owner: str, quantity: str, etas: str,
                 claim_bounty: str, placed_quantity: str, placed_at: int, price: str, percentClaimed: str,
                 totalFilled: str, percentFilled: str, orderbookAddress: str, status: str, output: str,
                 quote_asset: dict, base_asset: dict):
        self.tick_id = int(tick_id)
        self.order_id = int(order_id)
        self.order_direction = order_direction
        self.owner = owner
        self.quantity = Decimal(quantity)
        self.etas = Decimal(etas)
        self.claim_bounty = Decimal(claim_bounty)
        self.placed_quantity = Decimal(placed_quantity)
        self.placed_at = int(placed_at)
        self.price = Decimal(price)
        self.percent_claimed = Decimal(percentClaimed)  # Changed variable name
        self.total_filled = Decimal(totalFilled)
        self.percent_filled = Decimal(percentFilled)
        self.orderbook_address = orderbookAddress
        self.status = status
        self.output = Decimal(output)
        self.quote_asset = LimitOrderAsset(**quote_asset)
        self.base_asset = LimitOrderAsset(**base_asset)

    def validate(self, owner_address=None):
        # Check if order_id is non-negative
        assert self.order_id >= 0, f"Order ID {self.order_id} cannot be negative"

        # Check if order_direction is either "bid" or "ask"
        assert self.order_direction in ['bid', 'ask'], f"Order direction {self.order_direction} must be 'bid' or 'ask'"

        # Validate owner address (Osmosis address format)
        assert self.owner == owner_address, f"Owner address {self.owner} is invalid"

        # Check if quantity is non-negative
        assert self.quantity > 0, f"Quantity {self.quantity} cannot be negative"

        # Check if claim_bounty is non-negative
        assert self.claim_bounty > 0, f"Claim bounty {self.claim_bounty} cannot be negative"

        # Validate placed_quantity is non-negative
        assert self.placed_quantity > 0, f"Placed quantity {self.placed_quantity} cannot be negative"

        # Validate placed_at is a valid Unix timestamp
        assert 0 <= self.placed_at <= int(time.time()), f"Placed_at timestamp {self.placed_at} is invalid"

        # Check if price is positive
        assert self.price > 0, f"Price {self.price} must be positive"

        # Check if percent_claimed is between 0 and 100
        assert 0 <= self.percent_claimed <= 100, f"Percent claimed {self.percent_claimed} must be between 0 and 100"

        # Check if total_filled is non-negative
        assert self.total_filled >= 0, f"Total filled {self.total_filled} cannot be negative"

        # Check if percent_filled is between 0 and 100
        assert 0 <= self.percent_filled <= 100, f"Percent filled {self.percent_filled} must be between 0 and 100"

        # Ensure status is not empty
        assert self.status, "Status must not be empty"

        # Ensure orderbook_address is not empty
        assert self.orderbook_address, "Orderbook address must not be empty"

        # Check if output is non-negative
        assert self.output >= 0, f"Output {self.output} cannot be negative"

        # Validate quote_asset
        self.quote_asset.validate()

        # Validate base_asset
        self.base_asset.validate()

    @staticmethod
    def _is_valid_unix_timestamp(timestamp):
        try:
            datetime.utcfromtimestamp(int(timestamp))
            return True
        except (ValueError, OverflowError):
            return False


class OrderbookActiveOrdersResponse:
    def __init__(self, orders: List[dict], is_best_effort: bool):
        self.orders = [LimitOrder(**order) for order in orders]
        self.is_best_effort = is_best_effort

    def validate(self, owner_address):
        # Validate each order
        order_ids = set()
        for order in self.orders:
            order.validate(owner_address)

            # Ensure order_id is unique
            if order.order_id in order_ids:
                raise ValueError(f"Duplicate order_id found: {order.order_id}")
            order_ids.add(order.order_id)
