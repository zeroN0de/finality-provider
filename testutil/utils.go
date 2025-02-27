package testutil

import (
	"math/rand"
	"testing"

	sdkmath "cosmossdk.io/math"
	"github.com/golang/mock/gomock"

	"github.com/babylonchain/finality-provider/testutil/mocks"
	"github.com/babylonchain/finality-provider/types"
)

const TestPubRandNum = 25

func EmptyDescription() []byte {
	return []byte("empty description")
}

func ZeroCommissionRate() *sdkmath.LegacyDec {
	zeroCom := sdkmath.LegacyZeroDec()
	return &zeroCom
}

func PrepareMockedClientController(t *testing.T, r *rand.Rand, startHeight, currentHeight uint64) *mocks.MockClientController {
	ctl := gomock.NewController(t)
	mockClientController := mocks.NewMockClientController(ctl)

	for i := startHeight + 1; i <= currentHeight; i++ {
		resBlock := &types.BlockInfo{
			Height: currentHeight,
			Hash:   GenRandomByteArray(r, 32),
		}
		mockClientController.EXPECT().QueryBlock(i).Return(resBlock, nil).AnyTimes()
	}

	currentBlockRes := &types.BlockInfo{
		Height: currentHeight,
		Hash:   GenRandomByteArray(r, 32),
	}

	mockClientController.EXPECT().Close().Return(nil).AnyTimes()
	mockClientController.EXPECT().QueryBestBlock().Return(currentBlockRes, nil).AnyTimes()
	mockClientController.EXPECT().QueryActivatedHeight().Return(uint64(1), nil).AnyTimes()

	return mockClientController
}
