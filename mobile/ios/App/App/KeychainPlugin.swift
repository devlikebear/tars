import Foundation
import Capacitor
import Security

// TarsKeychain keeps the app's desktops and their device tokens in the iOS
// Keychain (frontend/console/src/lib/mobile/secureStore.ts). Items stay on
// this phone: they are not synced to iCloud and not restored to another
// phone from a backup, because a device token belongs to the phone it was
// issued to (docs/decisions/mobile-remote.md §4).
@objc(KeychainPlugin)
public class KeychainPlugin: CAPPlugin, CAPBridgedPlugin {
    public let identifier = "KeychainPlugin"
    public let jsName = "TarsKeychain"
    public let pluginMethods: [CAPPluginMethod] = [
        CAPPluginMethod(name: "get", returnType: CAPPluginReturnPromise),
        CAPPluginMethod(name: "set", returnType: CAPPluginReturnPromise),
        CAPPluginMethod(name: "remove", returnType: CAPPluginReturnPromise)
    ]

    private let service = "com.devlikebear.tars.mobile"

    private func itemQuery(_ key: String) -> [String: Any] {
        return [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: key
        ]
    }

    private func key(of call: CAPPluginCall) -> String? {
        guard let key = call.getString("key"), !key.isEmpty else {
            call.reject("key is required")
            return nil
        }
        return key
    }

    @objc func get(_ call: CAPPluginCall) {
        guard let key = key(of: call) else { return }
        var query = itemQuery(key)
        query[kSecReturnData as String] = true
        query[kSecMatchLimit as String] = kSecMatchLimitOne
        var item: CFTypeRef?
        let status = SecItemCopyMatching(query as CFDictionary, &item)
        if status == errSecItemNotFound {
            call.resolve(["value": NSNull()])
            return
        }
        guard status == errSecSuccess, let data = item as? Data, let value = String(data: data, encoding: .utf8) else {
            call.reject("keychain read failed", String(status))
            return
        }
        call.resolve(["value": value])
    }

    @objc func set(_ call: CAPPluginCall) {
        guard let key = key(of: call) else { return }
        guard let value = call.getString("value") else {
            call.reject("value is required")
            return
        }
        let attributes: [String: Any] = [
            kSecValueData as String: Data(value.utf8),
            kSecAttrAccessible as String: kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly
        ]
        var status = SecItemUpdate(itemQuery(key) as CFDictionary, attributes as CFDictionary)
        if status == errSecItemNotFound {
            let item = itemQuery(key).merging(attributes) { _, new in new }
            status = SecItemAdd(item as CFDictionary, nil)
        }
        if status == errSecSuccess {
            call.resolve()
        } else {
            call.reject("keychain write failed", String(status))
        }
    }

    @objc func remove(_ call: CAPPluginCall) {
        guard let key = key(of: call) else { return }
        let status = SecItemDelete(itemQuery(key) as CFDictionary)
        if status == errSecSuccess || status == errSecItemNotFound {
            call.resolve()
        } else {
            call.reject("keychain delete failed", String(status))
        }
    }
}
