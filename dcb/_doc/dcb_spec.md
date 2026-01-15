# DCB Specification

## Note

This document defines the minimal feature set an Event Store must provide to be DCB compliant.

While we introduce certain concepts and terminology, implementations are not required to use the same terms or function/field names — as long as they offer equivalent functionality. (e.g., read() could be getEvents() and
failIfEventsMatch could be referenceQuery if those make more sense in your API)

## Event Store Capabilities

An Event Store that supports DCB provides a way to:

- read Sequenced Events matching a Query, optionally starting from a specified Sequence Position
- append Event(s), optionally specifying an Append Condition to enforce consistency

## Reading Events

The Event Store...

- MUST provide a way to filter Events based on their Event Type and/or Tag (see Query)
- SHOULD provide a way to read Events from a given starting Sequence Position
- MAY provide further filter options, e.g. for ordering or to limit the number of Events to load at once

A typical interface for reading events (pseudo-code):

```
EventStore {
  read(query: Query, options?: ReadOptions): SequencedEvents
  // ...
}
```

Note: SequencedEvents represents some form of iterable or reactive stream of Sequenced Events.

## Writing Events

The Event Store...

- MUST provide a way to atomically persist one or more Event(s)
- MUST fail if the Event Store contains at least one Event matching the Append Condition, if specified

A typical interface for writing events (pseudo-code):

```
EventStore {
  // ...
  append(events: Events|Event, condition?: AppendCondition): void
}
```

## Concepts

### Query

The Query describes constraints that must be matched by Events in the Event Store. It effectively allows for filtering Events by their Type and/or Tags.

- MUST contain a set of Query Items with at least one item or represent a query that matches all Events
- All Query Items are effectively combined with an OR, e.g. adding an extra Query Item will likely result in more Events being returned

To differentiate the two query variants, dedicated factory methods might be helpful:

- Query.fromItems(items)
- Query.all()

### Query Item

Each item of a Query allows to target Events by their Type and/or Tags.

An Event, to match a specific Query Item, needs to have the following characteristics:

- the Type MUST match one of the provided Types of the Query Item
- the Tags MUST contain all of the Tags specified by the Query Item

#### Example

The following example query would match Events that are either...

- of type EventType1 OR EventType2
- tagged tag1 AND tag2
- of type EventType2 OR EventType3 AND tagged tag1 AND tag3

```
{
  "items": [
    {
      "types": ["EventType1", "EventType2"]
    },
    {
      "tags": ["tag1", "tag2"]
    },
    {
      "types": ["EventType2", "EventType3"],
      "tags": ["tag1", "tag3"]
    }
  ]
}
```

### Sequenced Event

Contains or embeds all information of the original Event and its Sequence Position that was added during the append() call.

- MUST contain the Sequence Position
- MUST contain the Event
- MAY contain further fields, like metadata defined by the Event Store

#### Example

A potential JSON representation of a Sequenced Event:

```
{
  "event": {
    ...
  },
  "position": 1234,
  ...
}
```

### Sequence Position

When an Event is appended, the Event Store assigns a Sequence Position to it.

It...

- MUST be unique in the Event Store
- MUST be monotonic increasing
- MAY contain gaps

### Events

A set of Event instances that is passed to the append() method of the Event Store.

It...

- MUST not be empty
- MUST be iterable, each iteration returning an Event

### Event

- MUST contain an Event Type
- MUST contain Event Data
- MAY contain Tags
- MAY contain further fields, like metadata defined by the client

#### Example

A potential JSON representation of an Event:

```
{
  "type": "SomeEventType",
  "data": "{\"some\":\"data\"}",
  "tags": ["tag1", "tag2"],
   ...
}
```

### Event Type

Type of the event used to filter Events in the Query.

### Event Data

Opaque payload of an Event.

### Tags

A set of Tags.

- SHOULD not contain multiple Tags with the same value

### Tag

A Tag can add domain-specific metadata to an event, allowing for custom partitioning. Usually, a Tag represents a concept of the domain, e.g. the type and id of an entity like product:p123.

- MAY represent a key/value pair such as product:123 but that is irrelevant to the Event Store

### Append Condition

The Append Condition is used to enforce consistency, ensuring that between the time of building the Decision Model and appending the events, no new events were stored by another client that match the same query.

It...

- MUST contain a failIfEventsMatch Query
- MAY contain an after Sequence Position

Notes:

- after represents the highest position the client was aware of while building the Decision Model.
- The Event Store MUST ignore the Events before the specified position while checking the condition for appending events.
- This number can be higher than the position of the last event matching the Query.
- When after is present, failIfEventsMatch is typically the same Query used when building the Decision Model, which guarantees that the Decision Model is still the same when we append new events.
- If omitted, no Events will be ignored, effectively failing if any Event matches the specified Query.

```
AppendCondition {
  failIfEventsMatch: Query
  after?: SequencePosition
}
```
