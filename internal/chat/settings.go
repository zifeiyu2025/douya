// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package chat

import (
	"douya/internal/secrets"
	"douya/internal/store"
)

// GetSetting 读取明文存储的设置值。
//
// 生活类比：就像去仓库管理员那里取一个标了名字的盒子（key）里的东西，
// 盒子里可能空（返回空字符串），也可能有内容（返回 value）。
func (s *Service) GetSetting(key string) (string, error) {
	return store.GetSetting(s.db, key)
}

// SetSetting 写入明文存储的设置值。
//
// 生活类比：把东西（value）放进标了名字的盒子（key）里，送到仓库管理员那里保管。
func (s *Service) SetSetting(key, value string) error {
	return store.SetSetting(s.db, key, value)
}

// GetEncryptedSetting 读取加密存储的设置值并自动解密。
//
// 行为说明：
//   - 当 Service 持有 cipher（即开启了加密）时，读取并解密
//   - 当 Service 未持有 cipher（如未启用加密）时，回退到明文读取
//
// 生活类比：像一个智能快递柜——
//   有钥匙（cipher）就帮你拆开加密包裹；
//   没钥匙时直接给你未加密的普通包裹。
func (s *Service) GetEncryptedSetting(key string) (string, error) {
	if s.cipher != nil {
		return store.GetEncryptedSetting(s.db, key, secrets.CipherKey(s.cipher))
	}
	return store.GetSetting(s.db, key)
}

// SetEncryptedSetting 加密后存储设置值。
//
// 行为说明：
//   - 当 Service 持有 cipher 时，加密后存储
//   - 当 Service 未持有 cipher 时，回退到明文存储
//
// 生活类比：寄快递时——
//   有加密箱（cipher）就把物品锁进加密箱再寄出；
//   没有加密箱就直接用普通包装寄出。
func (s *Service) SetEncryptedSetting(key, value string) error {
	if s.cipher != nil {
		return store.SetEncryptedSetting(s.db, key, value, secrets.CipherKey(s.cipher))
	}
	return store.SetSetting(s.db, key, value)
}
