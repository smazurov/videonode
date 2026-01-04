#!/bin/bash

# Default values
ENDPOINT="events"
FILTER=""
BASE_URL="http://localhost:8090/api"
AUTH="pinball:ilovepinball"

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --metrics)
            ENDPOINT="metrics"
            shift
            ;;
        --filter=*)
            FILTER="${1#*=}"
            shift
            ;;
        --help)
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --metrics           Monitor /api/metrics endpoint instead of /api/events"
            echo "  --filter=e1,e2,e3   Filter for specific event types (comma-separated)"
            echo "  --help              Show this help message"
            echo ""
            echo "Examples:"
            echo "  $0                                    # Monitor all events"
            echo "  $0 --metrics                          # Monitor metrics endpoint"
            echo "  $0 --filter=capture-success,capture-error  # Only show capture events"
            echo "  $0 --metrics --filter=system-metrics     # Only system metrics"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            echo "Use --help for usage information"
            exit 1
            ;;
    esac
done

# Build filter array
IFS=',' read -ra FILTER_ARRAY <<< "$FILTER"

# Function to check if event should be displayed
should_display_event() {
    local line="$1"
    
    # If no filter, display everything
    if [ -z "$FILTER" ]; then
        return 0
    fi
    
    # Check if line contains any of the filter terms
    for filter_term in "${FILTER_ARRAY[@]}"; do
        if [[ "$line" == *"\"event\":\"$filter_term\""* ]] || [[ "$line" == *"event: $filter_term"* ]]; then
            return 0
        fi
    done
    
    # Also display non-event lines (connection messages, etc)
    if [[ "$line" != *"event:"* ]] && [[ "$line" != *"\"event\":"* ]]; then
        return 0
    fi
    
    return 1
}

# Display configuration
echo "========================================="
echo "SSE Monitor Configuration:"
echo "  Endpoint: /api/$ENDPOINT"
if [ -n "$FILTER" ]; then
    echo "  Filter: $FILTER"
else
    echo "  Filter: none (showing all events)"
fi
echo "========================================="
echo ""

# Main monitoring loop
while true; do
    echo "[$(date '+%H:%M:%S')] Connecting to $BASE_URL/$ENDPOINT..."
    curl -N -u "$AUTH" "$BASE_URL/$ENDPOINT" 2>/dev/null | while IFS= read -r line; do
        if should_display_event "$line"; then
            echo "[$(date '+%H:%M:%S')] $line"
        fi
    done
    echo "[$(date '+%H:%M:%S')] Connection dropped, reconnecting in 2s..."
    sleep 2
done