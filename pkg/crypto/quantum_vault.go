package crypto

/*
#cgo CFLAGS: -I${SRCDIR}/../../libs/include
#cgo darwin,arm64 LDFLAGS: -L${SRCDIR}/../../libs/lib/darwin-arm64 -lquantum_vault
#cgo linux,amd64 LDFLAGS: -L${SRCDIR}/../../libs/lib/linux-amd64 -lquantum_vault

#include "quantum_vault.h"
#include <string.h>
*/
import "C"
import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"unsafe"
)

const (
	MLDSAPublicKeySize  = C.QV_MLDSA65_PK_SIZE
	MLDSASecretKeySize  = C.QV_MLDSA65_SK_SIZE
	MLDSASignatureSize  = C.QV_MLDSA65_SIG_SIZE
	HybridEncapsKeySize = C.QV_HYBRID_EK_SIZE
	HybridDecapsKeySize = C.QV_HYBRID_DK_SIZE
	HybridCiphertextSize = C.QV_HYBRID_CT_SIZE
	HybridSharedSecretSize = C.QV_HYBRID_SS_SIZE
)

type MLDSAKeyPair struct {
	PublicKey [MLDSAPublicKeySize]byte
	SecretKey [MLDSASecretKeySize]byte
}

type HybridKeyPair struct {
	EncapsKey [HybridEncapsKeySize]byte
	DecapsKey [HybridDecapsKeySize]byte
}

type HybridEncapsResult struct {
	SharedSecret [HybridSharedSecretSize]byte
	Ciphertext   [HybridCiphertextSize]byte
}

func qvError(rc C.QvError) error {
	switch rc {
	case C.QV_SUCCESS:
		return nil
	case C.QV_INVALID_PARAMETER:
		return fmt.Errorf("invalid parameter")
	case C.QV_RNG_FAILURE:
		return fmt.Errorf("RNG failure")
	case C.QV_MEMORY_ERROR:
		return fmt.Errorf("memory error")
	case C.QV_MLDSA_SIGNING_FAILED:
		return fmt.Errorf("ML-DSA signing failed")
	case C.QV_MLDSA_VERIFICATION_FAILED:
		return fmt.Errorf("ML-DSA verification failed")
	case C.QV_MLDSA_KEYGEN_FAILED:
		return fmt.Errorf("ML-DSA keygen failed")
	case C.QV_HYBRID_KEYGEN_FAILED:
		return fmt.Errorf("hybrid keygen failed")
	case C.QV_HYBRID_ENCAPS_FAILED:
		return fmt.Errorf("hybrid encaps failed")
	case C.QV_HYBRID_DECAPS_FAILED:
		return fmt.Errorf("hybrid decaps failed")
	default:
		return fmt.Errorf("quantum vault error: %d", rc)
	}
}

// GenerateMLDSAKeyPair generates a new ML-DSA-65 identity keypair.
func GenerateMLDSAKeyPair() (*MLDSAKeyPair, error) {
	var ckp C.QvMlDsaKeyPair
	rc := C.qv_mldsa65_keygen_random(&ckp)
	if err := qvError(rc); err != nil {
		return nil, fmt.Errorf("keygen: %w", err)
	}
	kp := &MLDSAKeyPair{}
	C.memcpy(unsafe.Pointer(&kp.PublicKey[0]), unsafe.Pointer(&ckp.pk.data[0]), C.size_t(MLDSAPublicKeySize))
	C.memcpy(unsafe.Pointer(&kp.SecretKey[0]), unsafe.Pointer(&ckp.sk.data[0]), C.size_t(MLDSASecretKeySize))
	C.qv_secure_zero((*C.uint8_t)(unsafe.Pointer(&ckp.sk.data[0])), C.size_t(MLDSASecretKeySize))
	return kp, nil
}

// Sign signs a message with ML-DSA-65 (randomized).
func Sign(sk *[MLDSASecretKeySize]byte, message []byte) ([MLDSASignatureSize]byte, error) {
	var csk C.QvMlDsaSecretKey
	C.memcpy(unsafe.Pointer(&csk.data[0]), unsafe.Pointer(&sk[0]), C.size_t(MLDSASecretKeySize))
	defer C.qv_secure_zero((*C.uint8_t)(unsafe.Pointer(&csk.data[0])), C.size_t(MLDSASecretKeySize))

	var csig C.QvMlDsaSignature
	var msgPtr *C.uint8_t
	if len(message) > 0 {
		msgPtr = (*C.uint8_t)(unsafe.Pointer(&message[0]))
	}
	rc := C.qv_mldsa65_sign_randomized(&csk, msgPtr, C.size_t(len(message)), &csig)
	if err := qvError(rc); err != nil {
		return [MLDSASignatureSize]byte{}, fmt.Errorf("sign: %w", err)
	}
	var sig [MLDSASignatureSize]byte
	C.memcpy(unsafe.Pointer(&sig[0]), unsafe.Pointer(&csig.data[0]), C.size_t(MLDSASignatureSize))
	return sig, nil
}

// Verify verifies an ML-DSA-65 signature.
func Verify(pk *[MLDSAPublicKeySize]byte, message []byte, sig *[MLDSASignatureSize]byte) bool {
	var cpk C.QvMlDsaPublicKey
	C.memcpy(unsafe.Pointer(&cpk.data[0]), unsafe.Pointer(&pk[0]), C.size_t(MLDSAPublicKeySize))
	var csig C.QvMlDsaSignature
	C.memcpy(unsafe.Pointer(&csig.data[0]), unsafe.Pointer(&sig[0]), C.size_t(MLDSASignatureSize))

	var msgPtr *C.uint8_t
	if len(message) > 0 {
		msgPtr = (*C.uint8_t)(unsafe.Pointer(&message[0]))
	}
	rc := C.qv_mldsa65_verify(&cpk, msgPtr, C.size_t(len(message)), &csig)
	return rc == C.QV_SUCCESS
}

// GenerateHybridKeyPair generates a hybrid ML-KEM-768 + X25519 keypair.
func GenerateHybridKeyPair() (*HybridKeyPair, error) {
	var ckp C.QvHybridKeyPair
	rc := C.qv_hybrid_keygen(&ckp)
	if err := qvError(rc); err != nil {
		return nil, fmt.Errorf("hybrid keygen: %w", err)
	}
	kp := &HybridKeyPair{}
	C.memcpy(unsafe.Pointer(&kp.EncapsKey[0]), unsafe.Pointer(&ckp.ek.data[0]), C.size_t(HybridEncapsKeySize))
	C.memcpy(unsafe.Pointer(&kp.DecapsKey[0]), unsafe.Pointer(&ckp.dk.data[0]), C.size_t(HybridDecapsKeySize))
	C.qv_secure_zero((*C.uint8_t)(unsafe.Pointer(&ckp.dk.data[0])), C.size_t(HybridDecapsKeySize))
	return kp, nil
}

// HybridEncaps performs hybrid encapsulation to produce a shared secret.
func HybridEncaps(ek *[HybridEncapsKeySize]byte) (*HybridEncapsResult, error) {
	var cek C.QvHybridEncapsKey
	C.memcpy(unsafe.Pointer(&cek.data[0]), unsafe.Pointer(&ek[0]), C.size_t(HybridEncapsKeySize))
	var cresult C.QvHybridEncapsResult
	rc := C.qv_hybrid_encaps(&cek, &cresult)
	if err := qvError(rc); err != nil {
		return nil, fmt.Errorf("hybrid encaps: %w", err)
	}
	result := &HybridEncapsResult{}
	C.memcpy(unsafe.Pointer(&result.SharedSecret[0]), unsafe.Pointer(&cresult.shared_secret[0]), C.size_t(HybridSharedSecretSize))
	C.memcpy(unsafe.Pointer(&result.Ciphertext[0]), unsafe.Pointer(&cresult.ciphertext.data[0]), C.size_t(HybridCiphertextSize))
	return result, nil
}

// HybridDecaps recovers the shared secret from a ciphertext.
func HybridDecaps(dk *[HybridDecapsKeySize]byte, ct *[HybridCiphertextSize]byte) ([HybridSharedSecretSize]byte, error) {
	var cdk C.QvHybridDecapsKey
	C.memcpy(unsafe.Pointer(&cdk.data[0]), unsafe.Pointer(&dk[0]), C.size_t(HybridDecapsKeySize))
	defer C.qv_secure_zero((*C.uint8_t)(unsafe.Pointer(&cdk.data[0])), C.size_t(HybridDecapsKeySize))
	var cct C.QvHybridCiphertext
	C.memcpy(unsafe.Pointer(&cct.data[0]), unsafe.Pointer(&ct[0]), C.size_t(HybridCiphertextSize))

	var ss [HybridSharedSecretSize]byte
	rc := C.qv_hybrid_decaps(&cdk, &cct, (*C.uint8_t)(unsafe.Pointer(&ss[0])))
	if err := qvError(rc); err != nil {
		return ss, fmt.Errorf("hybrid decaps: %w", err)
	}
	return ss, nil
}

// Fingerprint returns a short hex fingerprint of an ML-DSA public key.
func Fingerprint(pk *[MLDSAPublicKeySize]byte) string {
	h := sha256.Sum256(pk[:])
	return hex.EncodeToString(h[:8])
}

// Version returns the quantum vault library version.
func Version() string {
	return C.GoString(C.qv_version())
}
