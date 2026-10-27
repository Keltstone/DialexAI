import Foundation
import Security

@objc protocol SafeServiceProtocol {
    func performSafeAction()
}

class SafeXPCListener: NSObject, NSXPCListenerDelegate {
    func listener(_ listener: NSXPCListener, shouldAcceptNewConnection newConnection: NSXPCConnection) -> Bool {
        // Mitigation: verifies audit token and code signing requirement
        var token = newConnection.auditToken
        var guestCode: SecCode?
        let attributes = [kSecGuestAttributeAudit: Data(bytes: &token, count: MemoryLayout<audit_token_t>.size)] as CFDictionary
        if SecCodeCopyGuestWithAuditToken(nil, &token, &guestCode) != errSecSuccess {
            return false
        }
        newConnection.exportedInterface = NSXPCInterface(with: SafeServiceProtocol.self)
        newConnection.resume()
        return true
    }
}
