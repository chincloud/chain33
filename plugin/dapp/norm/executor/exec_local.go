// Copyright Fuzamei Corp. 2018 All Rights Reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package executor

import (
	pty "github.com/33cn/chain33/plugin/dapp/norm/types"
	"github.com/33cn/chain33/types"
)

// ExecLocal_Nput Action
func (n *Norm) ExecLocal_Nput(nput *pty.NormPut, tx *types.Transaction, receipt *types.ReceiptData, index int) (*types.LocalDBSet, error) {
	return nil, nil
}
