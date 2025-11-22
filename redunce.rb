class Redunce < Formula
  desc "Find potentially redundant code using vector similarity"
  homepage "https://github.com/ZachSnow/redunce"
  url "https://github.com/ZachSnow/redunce/archive/refs/tags/v0.1.0.tar.gz"
  sha256 "PUT_SHA256_HERE"
  license "MIT"

  depends_on "go" => :build

  # sqlite-vector extension for macOS
  resource "sqlite-vector-darwin" do
    url "https://github.com/sqliteai/sqlite-vector/releases/download/0.9.52/vector-apple-xcframework-0.9.52.zip"
    sha256 "PUT_SHA256_HERE"
  end

  def install
    # Install sqlite-vector extension to Homebrew's lib directory
    resource("sqlite-vector-darwin").stage do
      # Extract the dylib from the framework
      system "cp", "vector.xcframework/macos-arm64_x86_64/vector.framework/vector", "libvector.dylib"
      # Code sign it
      system "codesign", "--remove-signature", "libvector.dylib"
      system "codesign", "-s", "-", "libvector.dylib"
      # Install to Homebrew's lib directory so SQLite can find it
      lib.install "libvector.dylib"
    end

    # Build redunce with CGO enabled for SQLite extension loading
    ENV["CGO_CFLAGS"] = "-DSQLITE_ENABLE_LOAD_EXTENSION=1"
    system "go", "build", *std_go_args(ldflags: "-s -w")
  end

  def caveats
    <<~EOS
      The sqlite-vector extension has been installed to:
        #{lib}/libvector.dylib

      redunce will automatically find it in Homebrew's library path.
    EOS
  end

  test do
    system "#{bin}/redunce", "--help"
  end
end
