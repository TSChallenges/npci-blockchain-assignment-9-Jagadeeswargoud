package main

import (
	"encoding/json"
	"fmt"
	
	"time"

	"github.com/hyperledger/fabric-contract-api-go/contractapi"
)

type Loan struct {
	LoanID           string    `json:"loanId"`
	BorrowerID       string    `json:"borrowerId"`
	LenderID         string    `json:"lenderId"`
	Amount           float64   `json:"amount"`
	InterestRate     float64   `json:"interestRate"`
	Duration         int       `json:"duration"`
	Status           string    `json:"status"` // PENDING, APPROVED, ACTIVE, REPAID, DEFAULTED
	DisbursementDate string    `json:"disbursementDate"`
	RepaymentDue     float64   `json:"repaymentDue"`
	RemainingBalance float64   `json:"remainingBalance"`
	Collateral       string    `json:"collateral"`
	Defaulted        bool      `json:"defaulted"`
	AuditHistory     []string  `json:"auditHistory"`
	CreatedAt        string    `json:"createdAt"`
	DueDate          string    `json:"dueDate"`
}

type TokenBalance struct {
	Account string  `json:"account"`
	Balance float64 `json:"balance"`
}

type SmartContract struct {
	contractapi.Contract
}

// Initialize ledger with token balances
func (s *SmartContract) InitLedger(ctx contractapi.TransactionContextInterface) error {
	balances := []TokenBalance{
		{Account: "RBI", Balance: 1000000},
		{Account: "HDFC", Balance: 500000},
		{Account: "SBI", Balance: 500000},
	}

	for _, balance := range balances {
		balanceJSON, err := json.Marshal(balance)
		if err != nil {
			return err
		}
		err = ctx.GetStub().PutState(balance.Account, balanceJSON)
		if err != nil {
			return fmt.Errorf("failed to put to world state: %v", err)
		}
	}

	return nil
}

// Request a new loan
func (s *SmartContract) RequestLoan(
	ctx contractapi.TransactionContextInterface,
	loanID string,
	borrowerID string,
	amount float64,
	interestRate float64,
	duration int,
	collateral string,
) error {
	exists, err := s.LoanExists(ctx, loanID)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("loan %s already exists", loanID)
	}

	txTime, _ := ctx.GetStub().GetTxTimestamp()
	dueDate := time.Unix(txTime.GetSeconds(), 0).AddDate(0, duration, 0)

	loan := Loan{
		LoanID:       loanID,
		BorrowerID:   borrowerID,
		Amount:       amount,
		InterestRate: interestRate,
		Duration:     duration,
		Status:       "PENDING",
		RepaymentDue: amount * (1 + interestRate/100),
		RemainingBalance: amount * (1 + interestRate/100),
		Collateral:   collateral,
		Defaulted:    false,
		CreatedAt:    fmt.Sprintf("%d", txTime.GetSeconds()),
		DueDate:      dueDate.Format(time.RFC3339),
		AuditHistory: []string{
			fmt.Sprintf("Loan requested by %s (TxID: %s)", 
				borrowerID, 
				ctx.GetStub().GetTxID()),
		},
	}

	loanJSON, err := json.Marshal(loan)
	if err != nil {
		return err
	}

	return ctx.GetStub().PutState(loanID, loanJSON)
}

// Approve a loan request
func (s *SmartContract) ApproveLoan(
	ctx contractapi.TransactionContextInterface,
	loanID string,
	lenderID string,
) error {
	loan, err := s.GetLoan(ctx, loanID)
	if err != nil {
		return err
	}

	if loan.Status != "PENDING" {
		return fmt.Errorf("loan %s cannot be approved in current status: %s", loanID, loan.Status)
	}

	// Check lender balance
	lenderBalance, err := s.GetBalance(ctx, lenderID)
	if err != nil {
		return err
	}
	if lenderBalance < loan.Amount {
		return fmt.Errorf("lender %s has insufficient funds", lenderID)
	}

	// Update loan status
	loan.LenderID = lenderID
	loan.Status = "APPROVED"
	loan.AuditHistory = append(loan.AuditHistory, 
		fmt.Sprintf("Loan approved by %s (TxID: %s)", 
			lenderID, 
			ctx.GetStub().GetTxID()))

	loanJSON, err := json.Marshal(loan)
	if err != nil {
		return err
	}

	return ctx.GetStub().PutState(loanID, loanJSON)
}

// Disburse loan amount to borrower
func (s *SmartContract) DisburseLoan(
	ctx contractapi.TransactionContextInterface,
	loanID string,
) error {
	loan, err := s.GetLoan(ctx, loanID)
	if err != nil {
		return err
	}

	if loan.Status != "APPROVED" {
		return fmt.Errorf("loan %s cannot be disbursed in current status: %s", loanID, loan.Status)
	}

	// Transfer tokens from lender to borrower
	err = s.TransferTokens(ctx, loan.LenderID, loan.BorrowerID, loan.Amount)
	if err != nil {
		return err
	}

	// Update loan status
	loan.Status = "ACTIVE"
	txTime, _ := ctx.GetStub().GetTxTimestamp()
	loan.DisbursementDate = fmt.Sprintf("%d", txTime.GetSeconds())
	loan.AuditHistory = append(loan.AuditHistory, 
		fmt.Sprintf("Loan disbursed (TxID: %s)", 
			ctx.GetStub().GetTxID()))

	loanJSON, err := json.Marshal(loan)
	if err != nil {
		return err
	}

	return ctx.GetStub().PutState(loanID, loanJSON)
}

// Repay loan amount
func (s *SmartContract) RepayLoan(
	ctx contractapi.TransactionContextInterface,
	loanID string,
	amount float64,
) error {
	loan, err := s.GetLoan(ctx, loanID)
	if err != nil {
		return err
	}

	if loan.Status != "ACTIVE" {
		return fmt.Errorf("loan %s cannot be repaid in current status: %s", loanID, loan.Status)
	}

	// Check if repayment exceeds remaining balance
	if amount > loan.RemainingBalance {
		return fmt.Errorf("repayment amount exceeds remaining balance")
	}

	// Transfer tokens from borrower to lender
	err = s.TransferTokens(ctx, loan.BorrowerID, loan.LenderID, amount)
	if err != nil {
		return err
	}

	// Update loan status
	loan.RemainingBalance -= amount
	if loan.RemainingBalance <= 0 {
		loan.Status = "REPAID"
	}
	
	loan.AuditHistory = append(loan.AuditHistory, 
		fmt.Sprintf("Repayment of %f (TxID: %s)", 
			amount, 
			ctx.GetStub().GetTxID()))

	loanJSON, err := json.Marshal(loan)
	if err != nil {
		return err
	}

	return ctx.GetStub().PutState(loanID, loanJSON)
}

// Mark loan as defaulted
func (s *SmartContract) MarkAsDefaulted(
	ctx contractapi.TransactionContextInterface,
	loanID string,
) error {
	loan, err := s.GetLoan(ctx, loanID)
	if err != nil {
		return err
	}

	if loan.Status != "ACTIVE" {
		return fmt.Errorf("loan %s cannot be defaulted in current status: %s", loanID, loan.Status)
	}

	// Update loan status
	loan.Status = "DEFAULTED"
	loan.Defaulted = true
	loan.AuditHistory = append(loan.AuditHistory, 
		fmt.Sprintf("Loan marked as defaulted (TxID: %s)", 
			ctx.GetStub().GetTxID()))

	loanJSON, err := json.Marshal(loan)
	if err != nil {
		return err
	}

	return ctx.GetStub().PutState(loanID, loanJSON)
}

// ============== Token Functions (ERC20-like) ==============

func (s *SmartContract) GetBalance(
	ctx contractapi.TransactionContextInterface,
	account string,
) (float64, error) {
	balanceJSON, err := ctx.GetStub().GetState(account)
	if err != nil {
		return 0, fmt.Errorf("failed to read from world state: %v", err)
	}
	if balanceJSON == nil {
		return 0, fmt.Errorf("account %s does not exist", account)
	}

	var balance TokenBalance
	err = json.Unmarshal(balanceJSON, &balance)
	if err != nil {
		return 0, err
	}

	return balance.Balance, nil
}

func (s *SmartContract) TransferTokens(
	ctx contractapi.TransactionContextInterface,
	from string,
	to string,
	amount float64,
) error {
	// Get sender balance
	fromBalance, err := s.GetBalance(ctx, from)
	if err != nil {
		return err
	}

	// Check sufficient funds
	if fromBalance < amount {
		return fmt.Errorf("insufficient funds in account %s", from)
	}

	// Get recipient balance
	toBalance, err := s.GetBalance(ctx, to)
	if err != nil {
		// If recipient doesn't exist, create with 0 balance
		if err.Error() == fmt.Sprintf("account %s does not exist", to) {
			toBalance = 0
		} else {
			return err
		}
	}

	// Update balances
	fromBalance -= amount
	toBalance += amount

	// Save new balances
	err = s.UpdateBalance(ctx, from, fromBalance)
	if err != nil {
		return err
	}

	err = s.UpdateBalance(ctx, to, toBalance)
	if err != nil {
		return err
	}

	return nil
}

func (s *SmartContract) UpdateBalance(
	ctx contractapi.TransactionContextInterface,
	account string,
	newBalance float64,
) error {
	balance := TokenBalance{
		Account: account,
		Balance: newBalance,
	}

	balanceJSON, err := json.Marshal(balance)
	if err != nil {
		return err
	}

	return ctx.GetStub().PutState(account, balanceJSON)
}

// ============== Helper Functions ==============

func (s *SmartContract) LoanExists(
	ctx contractapi.TransactionContextInterface,
	loanID string,
) (bool, error) {
	loanJSON, err := ctx.GetStub().GetState(loanID)
	if err != nil {
		return false, fmt.Errorf("failed to read from world state: %v", err)
	}
	return loanJSON != nil, nil
}

func (s *SmartContract) GetLoan(
	ctx contractapi.TransactionContextInterface,
	loanID string,
) (*Loan, error) {
	loanJSON, err := ctx.GetStub().GetState(loanID)
	if err != nil {
		return nil, fmt.Errorf("failed to read from world state: %v", err)
	}
	if loanJSON == nil {
		return nil, fmt.Errorf("loan %s does not exist", loanID)
	}

	var loan Loan
	err = json.Unmarshal(loanJSON, &loan)
	if err != nil {
		return nil, err
	}

	return &loan, nil
}

func (s *SmartContract) GetLoanHistory(
	ctx contractapi.TransactionContextInterface,
	loanID string,
) ([]string, error) {
	loan, err := s.GetLoan(ctx, loanID)
	if err != nil {
		return nil, err
	}
	return loan.AuditHistory, nil
}

func (s *SmartContract) CheckLoanStatus(
	ctx contractapi.TransactionContextInterface,
	loanID string,
) (string, error) {
	loan, err := s.GetLoan(ctx, loanID)
	if err != nil {
		return "", err
	}
	return loan.Status, nil
}

func (s *SmartContract) AddCollateral(
	ctx contractapi.TransactionContextInterface,
	loanID string,
	collateral string,
) error {
	loan, err := s.GetLoan(ctx, loanID)
	if err != nil {
		return err
	}

	loan.Collateral = collateral
	loan.AuditHistory = append(loan.AuditHistory, 
		fmt.Sprintf("Collateral added: %s (TxID: %s)", 
			collateral, 
			ctx.GetStub().GetTxID()))

	loanJSON, err := json.Marshal(loan)
	if err != nil {
		return err
	}

	return ctx.GetStub().PutState(loanID, loanJSON)
}

func main() {
	chaincode, err := contractapi.NewChaincode(&SmartContract{})
	if err != nil {
		fmt.Printf("Error creating lending chaincode: %s", err.Error())
		return
	}

	if err := chaincode.Start(); err != nil {
		fmt.Printf("Error starting lending chaincode: %s", err.Error())
	}
}