package main

import (
	"crypto/sha256"
	"encoding/binary"
	"io"
	"math/big"

	"golang.org/x/crypto/hkdf"
)

const (
	K = 32
	L = K * 255
)

var (
	BigIntZero = big.NewInt(0)
)

func _flip_bits(in []byte) {
	for i := 0; i < len(in); i++ {
		in[i] = ^in[i]
	}
}
func _IKM_to_lamport_SK(IKM []byte, salt []byte) [][]byte {
	PRK := hkdf.Extract(sha256.New, []byte(IKM), []byte(salt))
	okmReader := hkdf.Expand(sha256.New, PRK, []byte(""))

	var lamport_SK [][]byte
	for i := 0; i < L/K; i++ {
		chunk := make([]byte, K)
		_, err := io.ReadFull(okmReader, chunk)
		if err != nil {
			panic(err)
		}

		lamport_SK = append(lamport_SK, chunk)
	}

	return lamport_SK
}

func _parent_SK_to_lamport_PK(parent_SK *big.Int, index uint32) []byte {
	salt := make([]byte, 4)

	binary.BigEndian.PutUint32(salt, index)
	IKM := make([]byte, K)
	parent_SK.FillBytes(IKM)

	lamport_0 := _IKM_to_lamport_SK(IKM, salt)
	_flip_bits(IKM)
	lamport_1 := _IKM_to_lamport_SK(IKM, salt)
	var lamport_PK []byte

	for i := 0; i < len(lamport_0); i++ {
		sum := sha256.Sum256(lamport_0[i])
		lamport_PK = append(lamport_PK, sum[:]...)
	}

	for i := 0; i < len(lamport_1); i++ {
		sum := sha256.Sum256(lamport_1[i])
		lamport_PK = append(lamport_PK, sum[:]...)
	}

	compressed_lamport_PK := sha256.Sum256([]byte(lamport_PK))
	return compressed_lamport_PK[:]
}

// 1. salt = "BLS-SIG-KEYGEN-SALT-"
// 2. SK = 0
// 3. while SK == 0:
// 4.     salt = H(salt)
// 5.     PRK = HKDF-Extract(salt, IKM || I2OSP(0, 1))
// 6.     OKM = HKDF-Expand(PRK, key_info || I2OSP(L, 2), L)
// 7.     SK = OS2IP(OKM) mod r
// 8. return SK
func _HKDF_mod_r(IKM []byte, key_info []byte) *big.Int {
	R, ok := new(big.Int).SetString("52435875175126190479447740508185965837690552500527637822603658699938581184513", 10)
	if !ok {
		panic("BLS 12-381 curve")
	}

	salt := []byte("BLS-SIG-KEYGEN-SALT-")
	SK := new(big.Int)

	sum := sha256.Sum256(salt)
	salt = sum[:]
	L := 48

	infoExtra := make([]byte, 2)
	binary.BigEndian.PutUint16(infoExtra, uint16(L))

	for SK.Cmp(BigIntZero) == 0 {
		// PRK = HKDF-Extract(salt, IKM || I2OSP(0, 1))
		ikm := make([]byte, len(IKM))
		copy(ikm, IKM)
		ikm = append(ikm, 0) // I20SP(0,1)

		PRK := hkdf.Extract(sha256.New, ikm, salt)

		//  OKM = HKDF-Expand(PRK, key_info || I2OSP(L, 2), L)
		info := make([]byte, len(key_info))
		copy(info, key_info)
		info = append(info, infoExtra...)
		okmReader := hkdf.Expand(sha256.New, PRK, info)

		OKM := make([]byte, L)
		_, err := io.ReadFull(okmReader, OKM)
		if err != nil {
			panic(err)
		}

		SK = new(big.Int).SetBytes(OKM)
		SK = SK.Mod(SK, R)
	}

	return SK
}

func _derive_child_SK(parent_SK *big.Int, index uint32) (child_SK *big.Int) {
	compressed_lamport_PK := _parent_SK_to_lamport_PK(parent_SK, index)
	return _HKDF_mod_r(compressed_lamport_PK, nil)
}

func _derive_master_SK(seed []byte) (SK *big.Int) {
	if len(seed) < 32 {
		panic("`len(seed)` should be greater than or equal to 32.")
	}

	return _HKDF_mod_r(seed, nil)
}
