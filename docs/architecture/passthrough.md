# Passthrough Module

Passtrhough module is the module that "passes through" the queries to the chain node via gRPC, aggregates results and returns to the client.

## Context

[Original ADR](https://www.notion.so/osmosiszone/ADR-014-Minimize-Latency-for-Geo-Distributed-Queries-a9d85415867941daa3436d3070a2a9b1)

For many user flows, the Osmosis FE app performs multiple queries to the chain via tRPC functions, where the results of some intermediary queries are prerequisites for others. Due to the deployment architecture and the geographical distance of the edge functions relative to the node, these intermediary query latencies become bottlenecks.

The new portfolio page [project](https://linear.app/osmosis/project/portfolio-page-01d03bea26cd) is delayed due to sluggish query performance, taking 4 seconds to load.

This module to move the client interacting with the node as close to the node as possible in a predictable manner to reduce intermediary latencies impacting the total time.

A [prototype](https://github.com/osmosis-labs/sqs/pull/291) exploiting this idea in SQS has been observed to reduce the total latency to near-instant, imperceptible levels.

### Portfolio Assets

#### Locks

Can be locked or unlocking. Can be regular token, gamm share or CL share.

If CL share, we want to skip because we get CL value from positions.

For token and gamm shares, there is no overlap with pooled assets or user-balances. As a result,
we count them as its own contribution to total assets.
