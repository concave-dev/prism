#!/bin/bash

# Final stress test for 1000 concurrent sandbox creations
# Uses timestamps to avoid name conflicts

set -e

TOTAL_SANDBOXES=${1:-1000}  # Default to 1000
PRISMCTL_PATH="./bin/prismctl"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}Starting MASSIVE Prism Stress Test${NC}"
echo -e "Total sandboxes: ${YELLOW}$TOTAL_SANDBOXES${NC}"
echo -e "Target: ${YELLOW}$(basename $PRISMCTL_PATH)${NC}"
echo ""

# Create results directory
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
RESULTS_DIR="massive_stress_test_$TIMESTAMP"
mkdir -p "$RESULTS_DIR"

echo -e "${BLUE}Results will be saved to: ${YELLOW}$RESULTS_DIR${NC}"
echo ""

# Function to create a single sandbox and measure timing
create_sandbox() {
    local index=$1
    local start_time=$(python3 -c "import time; print(int(time.time() * 1000))")
    
    # Create sandbox with unique name using timestamp and index
    local sandbox_name="load-test-${TIMESTAMP}-$index"
    
    # Capture both stdout and stderr, and measure timing
    local result
    local exit_code=0
    
    result=$($PRISMCTL_PATH sandbox create --name "$sandbox_name" 2>&1) || exit_code=$?
    
    local end_time=$(python3 -c "import time; print(int(time.time() * 1000))")
    local duration=$((end_time - start_time))
    
    # Log results
    if [ $exit_code -eq 0 ]; then
        echo "$index,$sandbox_name,$duration,success" >> "$RESULTS_DIR/results.csv"
        if (( index % 50 == 0 )); then
            echo -e "${GREEN}OK${NC} Sandbox $index created in ${duration}ms"
        fi
    else
        echo "$index,$sandbox_name,$duration,failed,$result" >> "$RESULTS_DIR/results.csv"
        echo -e "${RED}FAIL${NC} Sandbox $index failed in ${duration}ms"
    fi
}

# Create CSV header
echo "index,name,duration_ms,status,details" > "$RESULTS_DIR/results.csv"

echo -e "${BLUE}Starting massive sandbox creation wave...${NC}"
echo -e "${YELLOW}Progress indicators every 50 sandboxes${NC}"
echo ""

# Record overall start time
OVERALL_START=$(python3 -c "import time; print(int(time.time() * 1000))")

# Create sandboxes in parallel with controlled batching
BATCH_SIZE=50
for ((batch=0; batch*BATCH_SIZE < TOTAL_SANDBOXES; batch++)); do
    start_idx=$((batch * BATCH_SIZE + 1))
    end_idx=$(( (batch + 1) * BATCH_SIZE ))
    if (( end_idx > TOTAL_SANDBOXES )); then
        end_idx=$TOTAL_SANDBOXES
    fi
    
    # Launch batch in parallel
    for i in $(seq $start_idx $end_idx); do
        create_sandbox $i &
    done
    
    # Wait for current batch to complete
    wait
    
    # Progress update
    if (( batch % 5 == 0 )); then
        echo -e "${YELLOW}Completed batch $((batch+1)) (sandboxes 1-$end_idx)${NC}"
    fi
done

# Record overall end time
OVERALL_END=$(python3 -c "import time; print(int(time.time() * 1000))")
OVERALL_DURATION=$((OVERALL_END - OVERALL_START))

echo ""
echo -e "${GREEN}MASSIVE stress test completed!${NC}"
echo -e "Total time: ${YELLOW}${OVERALL_DURATION}ms${NC} ($(python3 -c "print(f'{$OVERALL_DURATION/1000:.2f}')") seconds)"

# Generate summary statistics
echo ""
echo -e "${BLUE}Generating statistics...${NC}"

# Count successes and failures
SUCCESSES=$(grep -c ",success" "$RESULTS_DIR/results.csv" || echo "0")
FAILURES=$(grep -c ",failed," "$RESULTS_DIR/results.csv" || echo "0")

if [ $TOTAL_SANDBOXES -gt 0 ]; then
    SUCCESS_RATE=$(python3 -c "print(f'{$SUCCESSES * 100 / $TOTAL_SANDBOXES:.1f}')")
else
    SUCCESS_RATE="0.0"
fi

echo -e "Successes: ${GREEN}$SUCCESSES${NC}"
echo -e "Failures: ${RED}$FAILURES${NC}"
echo -e "Success rate: ${YELLOW}$SUCCESS_RATE%${NC}"

# Calculate timing statistics from CSV
if [ $SUCCESSES -gt 0 ]; then
    echo ""
    echo -e "${BLUE}Timing Analysis (successful requests only):${NC}"
    
    # Extract duration column for successful requests and calculate stats
    grep ",success" "$RESULTS_DIR/results.csv" | cut -d',' -f3 | sort -n > "$RESULTS_DIR/durations.txt"
    
    MIN_TIME=$(head -n 1 "$RESULTS_DIR/durations.txt")
    MAX_TIME=$(tail -n 1 "$RESULTS_DIR/durations.txt")
    
    # Calculate average using Python for better precision
    TOTAL_TIME=$(awk '{sum += $1} END {print sum}' "$RESULTS_DIR/durations.txt")
    AVG_TIME=$(python3 -c "print(f'{$TOTAL_TIME / $SUCCESSES:.1f}')")
    
    # Calculate percentiles
    P50_LINE=$(python3 -c "print(int($SUCCESSES * 0.5))")
    P95_LINE=$(python3 -c "print(int($SUCCESSES * 0.95))")
    P99_LINE=$(python3 -c "print(int($SUCCESSES * 0.99))")
    
    P50_TIME=$(sed -n "${P50_LINE}p" "$RESULTS_DIR/durations.txt")
    P95_TIME=$(sed -n "${P95_LINE}p" "$RESULTS_DIR/durations.txt")
    P99_TIME=$(sed -n "${P99_LINE}p" "$RESULTS_DIR/durations.txt")
    
    echo -e "Min time: ${GREEN}${MIN_TIME}ms${NC}"
    echo -e "Max time: ${RED}${MAX_TIME}ms${NC}"
    echo -e "Average time: ${YELLOW}${AVG_TIME}ms${NC}"
    echo -e "P50 (median): ${BLUE}${P50_TIME}ms${NC}"
    echo -e "P95: ${YELLOW}${P95_TIME}ms${NC}"
    echo -e "P99: ${RED}${P99_TIME}ms${NC}"
    
    # Calculate throughput
    THROUGHPUT=$(python3 -c "print(f'{$SUCCESSES * 1000 / $OVERALL_DURATION:.2f}')")
    echo -e "Throughput: ${GREEN}${THROUGHPUT} sandboxes/second${NC}"
    
    # Queuing analysis
    echo ""
    echo -e "${BLUE}Queuing Analysis:${NC}"
    echo -e "Time spread (max-min): ${YELLOW}$((MAX_TIME - MIN_TIME))ms${NC}"
    echo -e "P99-P50 spread: ${YELLOW}$((P99_TIME - P50_TIME))ms${NC}"
    
    if (( MAX_TIME - MIN_TIME > 1000 )); then
        echo -e "${RED}WARNING: Significant queuing detected!${NC} (>1s spread)"
    elif (( MAX_TIME - MIN_TIME > 500 )); then
        echo -e "${YELLOW}WARNING: Moderate queuing detected${NC} (>500ms spread)"
    else
        echo -e "${GREEN}OK: Minimal queuing${NC} (<500ms spread)"
    fi
fi

echo ""
echo -e "${BLUE}Results saved to:${NC}"
echo -e "  Raw data: ${YELLOW}$RESULTS_DIR/results.csv${NC}"
echo -e "  Durations: ${YELLOW}$RESULTS_DIR/durations.txt${NC}"

# Final cluster state
echo ""
echo -e "${BLUE}Final cluster state:${NC}"
TOTAL_SANDBOXES_FINAL=$($PRISMCTL_PATH sandbox ls | grep -c "load-test-" || echo "0")
echo -e "Total sandboxes created: ${GREEN}$TOTAL_SANDBOXES_FINAL${NC}"

echo ""
echo -e "${BLUE}Summary:${NC}"
echo -e "• Attempted: ${YELLOW}$TOTAL_SANDBOXES${NC} sandboxes"
echo -e "• Succeeded: ${GREEN}$SUCCESSES${NC} (${SUCCESS_RATE}%)"
echo -e "• Throughput: ${GREEN}${THROUGHPUT}${NC} req/s"
echo -e "• Duration: ${YELLOW}$(python3 -c "print(f'{$OVERALL_DURATION/1000:.2f}')") seconds${NC}"
