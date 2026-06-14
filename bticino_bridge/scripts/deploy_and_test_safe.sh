#!/bin/bash

echo "=== BTicino Safe Testing Script ==="
echo "This script will test our Go OpenWebNet client safely"
echo "Only read-only commands will be tested initially"

# Configuration
BTICINO_HOST="bticino"
BTICINO_USER="root2"
BTICINO_KEY="~/.ssh/llave_broker"
REMOTE_PATH="/home/bticino/cfg/extra"
BINARY_NAME="bticino_test_arm"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${YELLOW}Step 1: Checking BTicino connection...${NC}"
if ! ssh -i "$BTICINO_KEY" -o ConnectTimeout=5 "$BTICINO_USER@$BTICINO_HOST" 'echo "Connection OK"' 2>/dev/null; then
    echo -e "${RED}❌ Cannot connect to BTicino. Check SSH configuration.${NC}"
    exit 1
fi
echo -e "${GREEN}✅ BTicino connection OK${NC}"

echo -e "${YELLOW}Step 2: Checking if binary exists...${NC}"
if [ ! -f "./build/$BINARY_NAME" ]; then
    echo -e "${RED}❌ Binary not found. Please compile first with: make build-arm${NC}"
    exit 1
fi
echo -e "${GREEN}✅ Binary found${NC}"

echo -e "${YELLOW}Step 3: Deploying test client to BTicino...${NC}"
scp -i "$BTICINO_KEY" -O "./build/$BINARY_NAME" "$BTICINO_USER@$BTICINO_HOST:$REMOTE_PATH/" || {
    echo -e "${RED}❌ Failed to deploy binary${NC}"
    exit 1
}
echo -e "${GREEN}✅ Binary deployed${NC}"

echo -e "${YELLOW}Step 4: Setting permissions...${NC}"
ssh -i "$BTICINO_KEY" "$BTICINO_USER@$BTICINO_HOST" "chmod +x $REMOTE_PATH/$BINARY_NAME" || {
    echo -e "${RED}❌ Failed to set permissions${NC}"
    exit 1
}
echo -e "${GREEN}✅ Permissions set${NC}"

echo -e "${YELLOW}Step 5: Running basic connectivity test...${NC}"
echo "Testing OpenWebNet port 30006 accessibility..."
ssh -i "$BTICINO_KEY" "$BTICINO_USER@$BTICINO_HOST" "timeout 5 nc -z localhost 30006 && echo 'Port 30006 is accessible' || echo 'Port 30006 is not accessible'"

echo -e "${YELLOW}Step 6: Running safe commands test...${NC}"
echo "This will run automated safe tests (heartbeat, status queries)"
echo "Press CTRL+C to cancel, or ENTER to continue..."
read -r

# Create automated test script
cat > temp_test_script.txt << 'EOF'
#!/bin/sh
cd /home/bticino/cfg/extra
echo "=== Starting BTicino OpenWebNet Safe Tests ==="
echo "1" | timeout 10 ./bticino_test_arm
echo ""
echo "2" | timeout 10 ./bticino_test_arm  
echo ""
echo "3" | timeout 10 ./bticino_test_arm
echo ""
echo "=== Safe tests completed ==="
EOF

scp -i "$BTICINO_KEY" -O temp_test_script.txt "$BTICINO_USER@$BTICINO_HOST:$REMOTE_PATH/run_safe_tests.sh"
rm temp_test_script.txt

ssh -i "$BTICINO_KEY" "$BTICINO_USER@$BTICINO_HOST" "chmod +x $REMOTE_PATH/run_safe_tests.sh && $REMOTE_PATH/run_safe_tests.sh"

echo -e "${GREEN}=== Safe testing completed! ===\n${NC}"
echo "To run interactive tests:"
echo "ssh -i $BTICINO_KEY $BTICINO_USER@$BTICINO_HOST"
echo "cd $REMOTE_PATH && ./bticino_test_arm"

echo -e "\n${YELLOW}Available next steps:${NC}"
echo "1. Run interactive test session"
echo "2. Test door status queries"
echo "3. Test audio/SIP status queries"
echo "4. Monitor OpenWebNet traffic"
echo ""
echo -e "${RED}⚠️  IMPORTANT: Only use read-only commands unless you're certain!${NC}"