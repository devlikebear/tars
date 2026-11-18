import UIKit
import Capacitor

class SceneDelegate: UIResponder, UIWindowSceneDelegate {
    var window: UIWindow?

    func scene(_ scene: UIScene, willConnectTo session: UISceneSession, options connectionOptions: UIScene.ConnectionOptions) {
        guard let windowScene = scene as? UIWindowScene else { return }

        window = UIWindow(windowScene: windowScene)
        window?.rootViewController = TarsBridgeViewController()
        window?.makeKeyAndVisible()

        SceneDelegateProxy.shared.scene(scene, willConnectTo: session, options: connectionOptions)
    }

    func scene(_ scene: UIScene, openURLContexts URLContexts: Set<UIOpenURLContext>) {
        SceneDelegateProxy.shared.scene(scene, openURLContexts: URLContexts)
    }

    func scene(_ scene: UIScene, continue userActivity: NSUserActivity) {
        SceneDelegateProxy.shared.scene(scene, continue: userActivity)
    }
}

// TarsBridgeViewController registers the app's own plugins, which live in
// this target rather than in an npm package.
class TarsBridgeViewController: CAPBridgeViewController {
    private let launchCover = LaunchMarkView()
    private var loading: NSKeyValueObservation?

    override open func capacitorDidLoad() {
        bridge?.registerPluginInstance(KeychainPlugin())

        // The web view takes a moment to start before the page draws
        // anything. Until the page has loaded (its script has mounted the
        // app by then), a native cover shows the launch image's mark in the
        // same place, so a cold start goes launch image -> mark -> the app,
        // cross-fading at the end, with no blank screen in between.
        launchCover.frame = view.bounds
        launchCover.autoresizingMask = [.flexibleWidth, .flexibleHeight]
        view.addSubview(launchCover)
        loading = webView?.observe(\.isLoading, options: [.new]) { [weak self] webView, _ in
            guard !webView.isLoading else { return }
            DispatchQueue.main.async { self?.dropLaunchCover() }
        }
    }

    private func dropLaunchCover() {
        loading = nil
        UIView.animate(withDuration: 0.15, animations: { self.launchCover.alpha = 0 }, completion: { _ in
            self.launchCover.removeFromSuperview()
        })
    }
}

// LaunchMarkView draws desktop/internal/icon Launch as iOS shows the launch
// image: the page colour, and the ring in a square filling the screen's
// longer side, its radius 4.5% of that side.
final class LaunchMarkView: UIView {
    private let ring = CAShapeLayer()
    private let dot = CAShapeLayer()

    override init(frame: CGRect) {
        super.init(frame: frame)
        isUserInteractionEnabled = false
        backgroundColor = UIColor(red: 0x0d / 255, green: 0x0f / 255, blue: 0x11 / 255, alpha: 1)
        let signal = UIColor(red: 0x3e / 255, green: 0xe0 / 255, blue: 0x7f / 255, alpha: 1).cgColor
        ring.fillColor = UIColor.clear.cgColor
        ring.strokeColor = signal
        dot.fillColor = signal
        layer.addSublayer(ring)
        layer.addSublayer(dot)
    }

    required init?(coder: NSCoder) {
        fatalError("LaunchMarkView is made in code")
    }

    override func layoutSubviews() {
        super.layoutSubviews()
        let side = max(bounds.width, bounds.height)
        let centre = CGPoint(x: bounds.midX, y: bounds.midY)
        ring.lineWidth = side * 0.012
        ring.path = UIBezierPath(arcCenter: centre, radius: side * 0.045, startAngle: 0, endAngle: .pi * 2, clockwise: true).cgPath
        dot.path = UIBezierPath(arcCenter: centre, radius: side * 0.015, startAngle: 0, endAngle: .pi * 2, clockwise: true).cgPath
    }
}
