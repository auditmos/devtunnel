class Devtunnel < Formula
  desc "Open-source ngrok alternative. Single binary, zero deps, instant setup"
  homepage "https://github.com/auditmos/devtunnel"
  version "0.1.0"
  license "MIT"

  on_macos do
    on_arm do
      url "https://github.com/auditmos/devtunnel/releases/download/v#{version}/devtunnel-darwin-arm64"
      sha256 "PLACEHOLDER_SHA256_DARWIN_ARM64"

      def install
        bin.install "devtunnel-darwin-arm64" => "devtunnel"
      end
    end

    on_intel do
      url "https://github.com/auditmos/devtunnel/releases/download/v#{version}/devtunnel-darwin-amd64"
      sha256 "PLACEHOLDER_SHA256_DARWIN_AMD64"

      def install
        bin.install "devtunnel-darwin-amd64" => "devtunnel"
      end
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/auditmos/devtunnel/releases/download/v#{version}/devtunnel-linux-arm64"
      sha256 "PLACEHOLDER_SHA256_LINUX_ARM64"

      def install
        bin.install "devtunnel-linux-arm64" => "devtunnel"
      end
    end

    on_intel do
      url "https://github.com/auditmos/devtunnel/releases/download/v#{version}/devtunnel-linux-amd64"
      sha256 "PLACEHOLDER_SHA256_LINUX_AMD64"

      def install
        bin.install "devtunnel-linux-amd64" => "devtunnel"
      end
    end
  end

  test do
    assert_match "devtunnel version", shell_output("#{bin}/devtunnel --version")
  end
end
