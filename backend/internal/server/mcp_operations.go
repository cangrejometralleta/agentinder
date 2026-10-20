package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"agentinder/backend/internal/store"
)

const (
	minOperationIDLength = 16
	maxOperationIDLength = 128
	operationReceiptTTL  = 30 * 24 * time.Hour
	protectedResponseTTL = 24 * time.Hour
	maxOperationReceipts = 20_000
)

var (
	errInvalidOperationID = errors.New("operationId debe tener entre 16 y 128 caracteres")
	errOperationConflict  = errors.New("operationId ya se usó con otros argumentos")
	errOperationCapacity  = errors.New("no se pueden aceptar más operaciones idempotentes ahora")
)

func validateOperationID(operationID string) error {
	if len(operationID) < minOperationIDLength || len(operationID) > maxOperationIDLength || strings.TrimSpace(operationID) != operationID {
		return errInvalidOperationID
	}
	return nil
}

func operationReceiptKey(ownerID, tool, operationID string) string {
	return hashOperationValue(ownerID + "\x00" + tool + "\x00" + operationID)
}

func hashOperationValue(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func (s *Server) applyIdempotentOperation(data *store.State, ownerID, tool, operationID string, input, output any, apply func() error, protectResponse bool) error {
	if err := validateOperationID(operationID); err != nil {
		return err
	}
	encodedInput, err := json.Marshal(input)
	if err != nil {
		return err
	}
	fingerprint := hashOperationValue(string(encodedInput))
	key := operationReceiptKey(ownerID, tool, operationID)
	now := time.Now().UTC()

	if receipt, exists := data.Operations[key]; exists {
		if now.Before(receipt.ExpiresAt) {
			if receipt.Fingerprint != fingerprint {
				return errOperationConflict
			}
			response := receipt.Response
			if receipt.Encrypted {
				response, err = s.openOperationResponse(key, response)
				if err != nil {
					return errors.New("no se pudo recuperar la respuesta de la operación")
				}
			}
			return json.Unmarshal(response, output)
		}
		delete(data.Operations, key)
	}

	if len(data.Operations) >= maxOperationReceipts {
		for receiptKey, receipt := range data.Operations {
			if !now.Before(receipt.ExpiresAt) {
				delete(data.Operations, receiptKey)
			}
		}
		if len(data.Operations) >= maxOperationReceipts {
			return errOperationCapacity
		}
	}

	if err = apply(); err != nil {
		return err
	}
	response, err := json.Marshal(output)
	if err != nil {
		return err
	}
	if protectResponse {
		response, err = s.sealOperationResponse(key, response)
		if err != nil {
			return err
		}
	}
	expiresAt := now.Add(operationReceiptTTL)
	if protectResponse {
		expiresAt = now.Add(protectedResponseTTL)
	}
	data.Operations[key] = store.OperationReceipt{
		Fingerprint: fingerprint,
		Response:    response,
		Encrypted:   protectResponse,
		ExpiresAt:   expiresAt,
	}
	return nil
}

func (s *Server) sealOperationResponse(key string, response []byte) ([]byte, error) {
	return s.tokens.Seal(response, []byte(key))
}

func (s *Server) openOperationResponse(key string, sealed []byte) ([]byte, error) {
	return s.tokens.Open(sealed, []byte(key))
}
