// server.js
const express = require('express');
const { Gateway, Wallets } = require('fabric-network');
const path = require('path');
const fs = require('fs');

const app = express();
app.use(express.json());

// Connect to the network
async function connectNetwork(userId) {
    const walletPath = path.join(process.cwd(), 'wallet');
    const wallet = await Wallets.newFileSystemWallet(walletPath);
    
    const gateway = new Gateway();
    const connectionProfile = JSON.parse(fs.readFileSync('connection.json', 'utf8'));
    
    await gateway.connect(connectionProfile, {
        wallet,
        identity: userId,
        discovery: { enabled: true, asLocalhost: true }
    });
    
    return gateway.getNetwork('mychannel');
}

// API endpoint to request a loan
app.post('/api/loans/request', async (req, res) => {
    try {
        const network = await connectNetwork(req.body.userId);
        const contract = network.getContract('lending');
        
        await contract.submitTransaction('RequestLoan', 
            req.body.loanId, 
            req.body.borrowerId, 
            req.body.amount.toString(), 
            req.body.interestRate.toString(), 
            req.body.duration.toString(), 
            req.body.collateral || '');
            
        res.json({ success: true });
    } catch (error) {
        res.status(500).json({ error: error.message });
    }
});

// Additional API endpoints for loan operations
// ...

app.listen(3000, () => console.log('Server running on port 3000'));