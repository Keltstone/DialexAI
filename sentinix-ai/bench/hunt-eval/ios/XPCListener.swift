import Foundation

@objc protocol ServiceProtocol {
    func performPrivilegedAction()
}

class XPCListener: NSObject, NSXPCListenerDelegate {
    func listener(_ listener: NSXPCListener, shouldAcceptNewConnection newConnection: NSXPCConnection) -> Bool {
        // Vulnerable: accepts all incoming connections without checking code signing or audit token
        newConnection.exportedInterface = NSXPCInterface(with: ServiceProtocol.self)
        newConnection.resume()
        return true
    }
}
