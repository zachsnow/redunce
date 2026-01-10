class Redunce < Formula
  desc "Find potentially redundant code using vector similarity"
  homepage "https://github.com/ZachSnow/redunce"
  url "https://github.com/ZachSnow/redunce/archive/refs/tags/v0.1.0.tar.gz"
  sha256 "0019dfc4b32d63c1392aa264aed2253c1e0c2fb09216f8e2cc269bbfb8bb49b5"
  license "MIT"

  depends_on "go" => :build

  # sqlite-vector extension for macOS
  resource "sqlite-vector-darwin" do
    url "https://github.com/sqliteai/sqlite-vector/releases/download/0.9.52/vector-apple-xcframework-0.9.52.zip"
    sha256 "7bcdc7b53474c3a4ce56d6d0720fa78a1fc1b836c0bbafbaa98593180ffba0d7"
  end

  def install
    # Install sqlite-vector extension to Homebrew's lib directory
    resource("sqlite-vector-darwin").stage do
      # Find the vector binary (Homebrew strips top-level directory when staging)
      vector_path = Dir.glob("**/macos-arm64_x86_64/vector.framework/vector").first
      odie "Could not find sqlite-vector binary" if vector_path.nil?

      # Copy and code sign
      system "cp", vector_path, "libvector.dylib"
      system "codesign", "--remove-signature", "libvector.dylib"
      system "codesign", "-s", "-", "libvector.dylib"
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

      Default ignore patterns are embedded in the binary.
      To customize, create an ignore file at one of these locations:
        ~/.config/redunce/ignore  (recommended, XDG-compliant)
        ~/.redunceignore          (alternative, simpler)

      Ignore files are combined in order: embedded defaults -> user global -> project
      Use !pattern to un-ignore patterns from earlier sources.
      Use --no-default-ignore to skip embedded defaults entirely.

      Claude Code Integration:
        To install Claude Code slash commands:
          redunce --install-claude-commands      # User-level (all projects)
          redunce --install-claude-commands .    # Project-level (this project only)

        This installs /redunce and /redunce-approve commands for interactive
        and auto-approve code deduplication workflows.
    EOS
  end

  test do
    system "#{bin}/redunce", "--help"
  end
end
