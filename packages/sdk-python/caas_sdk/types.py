"""Type definitions for AAGFE SDK."""

from enum import IntEnum


class EntityType(IntEnum):
    UNSPECIFIED = 0
    HUMAN = 1
    ORGANIZATION = 2
    DEVICE = 3
    SERVICE = 4
    AI_AGENT = 5
    AUTONOMOUS_SYSTEM = 6


class LifecycleState(IntEnum):
    UNSPECIFIED = 0
    PENDING = 1
    ACTIVE = 2
    SUSPENDED = 3
    REVOKED = 4
    ARCHIVED = 5


class AuthorityTier(IntEnum):
    INDIVIDUAL = 1
    INSTITUTIONAL = 2
    REGULATORY = 3
    JUDICIAL = 4
    SOVEREIGN_EMERGENCY = 5


class VoteType(IntEnum):
    APPROVE = 1
    REJECT = 2
    ESCALATE = 3
    ABSTAIN = 4
