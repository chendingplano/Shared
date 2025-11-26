/////////////////////////////////////////////////////////////
// Source: https://skoredin.pro/blog/golang/event-sourcing-go
// Description: An implementation of Event Sourcing
// Created: 2025/11/23 by Chen Ding
// Below are the results:
//	Metric				Before (CRUD)	After (Event Sourcing)
//	Write throughput	1K/sec			10K/sec
//	Read latency p99	5ms				2ms (projections)
//	Audit completeness	60%				100%
//	Debug time			Hours			Minutes (replay)
//	Storage cost		$1K/month		$3-5K/month
// The cost is: the monthly bills!
/////////////////////////////////////////////////////////////

package stores

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

type EventStore struct {
    db *sql.DB
}

func (es *EventStore) SaveEvents(
			ctx context.Context, 
			aggregateID, 
			aggregateType string, 
			events []ApiTypes.StoredEvent, 
			expectedVersion int) error {
    tx, err := es.db.BeginTx(ctx, nil)
    if err != nil {
        return err
    }
    defer tx.Rollback()
    
    // Check optimistic concurrency
    var currentVersion int
    err = tx.QueryRow(`
        SELECT COALESCE(MAX(event_version), 0) 
        FROM events 
        WHERE aggregate_id = $1`,
        aggregateID,
    ).Scan(&currentVersion)
    
    if err != nil {
        return err
    }
    
    if currentVersion != expectedVersion {
        return fmt.Errorf("concurrency conflict: expected version %d, got %d", 
            expectedVersion, currentVersion)
    }
    
    // Save events
    version := expectedVersion
    for _, event := range events {
        version++
        
        eventData, err := json.Marshal(event)
        if err != nil {
            return err
        }
        
        metadata := map[string]interface{}{
            "user_id":     ctx.Value("user_id"),
            "trace_id":    ctx.Value("trace_id"),
            "source":      ctx.Value("source"),
        }
        metadataJSON, _ := json.Marshal(metadata)
        
        _, err = tx.Exec(`
            INSERT INTO events (
                aggregate_id, aggregate_type, event_type, 
                event_version, event_data, metadata, occurred_at
            ) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
            aggregateID,
            aggregateType,
            event.EventType,
            version,
            eventData,
            metadataJSON,
            event.GetOccurredAt(),
        )
        
        if err != nil {
            return err
        }
    }
    
    return tx.Commit()
}

func (es *EventStore) GetEvents(ctx context.Context, aggregateID string, fromVersion int) ([]ApiTypes.StoredEvent, error) {
    rows, err := es.db.QueryContext(ctx, `
        SELECT 
            id, aggregate_id, aggregate_type, event_type,
            event_version, event_data, metadata, 
            occurred_at, recorded_at
        FROM events
        WHERE aggregate_id = $1 AND event_version > $2
        ORDER BY event_version`,
        aggregateID, fromVersion,
    )
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    
    var events []ApiTypes.StoredEvent
    for rows.Next() {
        var e ApiTypes.StoredEvent
        err := rows.Scan(
            &e.EventID, &e.AggregateID, &e.AggregateType,
            &e.EventType, &e.EventVersion, &e.EventData,
            &e.Metadata, &e.OccurredAt, &e.RecordedAt,
        )
        if err != nil {
            return nil, err
        }
        events = append(events, e)
    }
    
    return events, nil
}