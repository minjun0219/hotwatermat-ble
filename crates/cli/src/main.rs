//! 온수매트(KDO_HotWaterMat) BLE 제어 CLI의 진입점.

mod ble;
mod cli;
mod config;

#[tokio::main]
async fn main() {
    if let Err(e) = cli::run().await {
        eprintln!("Error: {e:#}");
        std::process::exit(1);
    }
}
