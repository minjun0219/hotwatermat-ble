//! 온수매트 BLE 제어용 CLI.
//!
//! 명령: scan / status / temp / on / off / setup / version.
//! 기기 주소와 인증 키는 플래그, 환경변수(HOTWATERMAT_ADDRESS / HOTWATERMAT_DEVICE_GID),
//! 설정 파일, 자동 스캔 순으로 결정된다.

use std::collections::HashMap;
use std::time::Duration;

use anyhow::{bail, Context, Result};
use clap::{Parser, Subcommand};

use crate::ble::{self, ScanResult};
use crate::config::{self, DeviceConfig};

/// BLE 기기 스캔 시간.
const SCAN_TIMEOUT: Duration = Duration::from_secs(5);
/// 명령 전송 후 매트가 처리할 시간을 확보하기 위한 대기 시간.
const POST_COMMAND_DELAY: Duration = Duration::from_secs(2);
/// STATUS 수신 대기 시간.
const STATUS_TIMEOUT: Duration = Duration::from_secs(5);

#[derive(Parser)]
#[command(
    name = "hotwatermat-ble",
    version,
    about = "Control a KDO_HotWaterMat device via Bluetooth Low Energy"
)]
pub struct Cli {
    /// BLE device address (or HOTWATERMAT_ADDRESS env)
    #[arg(long, global = true, env = "HOTWATERMAT_ADDRESS")]
    address: Option<String>,

    /// Device authentication key hex (or HOTWATERMAT_DEVICE_GID env)
    #[arg(long = "device-gid", global = true, env = "HOTWATERMAT_DEVICE_GID")]
    device_gid: Option<String>,

    /// Enable debug output
    #[arg(long, global = true)]
    debug: bool,

    #[command(subcommand)]
    command: Commands,
}

#[derive(Subcommand)]
enum Commands {
    /// Print version information
    Version,
    /// Scan for BLE hot water mat devices
    Scan,
    /// Show current mat status
    Status,
    /// Set target temperature
    Temp {
        /// Left side target temperature (28.0-48.0)
        #[arg(long)]
        left: Option<f64>,
        /// Right side target temperature (28.0-48.0)
        #[arg(long)]
        right: Option<f64>,
        /// Verify status after command
        #[arg(long)]
        verify: bool,
    },
    /// Power on the mat
    On {
        /// Verify status after command
        #[arg(long)]
        verify: bool,
    },
    /// Power off the mat
    Off {
        /// Verify status after command
        #[arg(long)]
        verify: bool,
    },
    /// Interactive setup: scan, pair, and save device config
    Setup {
        /// Clear cached device config
        #[arg(long)]
        reset: bool,
    },
}

/// CLI 진입점. 인자를 파싱하고 해당 명령을 실행한다.
pub async fn run() -> Result<()> {
    let cli = Cli::parse();
    match &cli.command {
        Commands::Version => {
            println!("hotwatermat-ble {}", env!("CARGO_PKG_VERSION"));
            Ok(())
        }
        Commands::Scan => cmd_scan().await,
        Commands::Status => cmd_status(&cli).await,
        Commands::Temp {
            left,
            right,
            verify,
        } => cmd_temp(&cli, *left, *right, *verify).await,
        Commands::On { verify } => cmd_on(&cli, *verify).await,
        Commands::Off { verify } => cmd_off(&cli, *verify).await,
        Commands::Setup { reset } => cmd_setup(&cli, *reset).await,
    }
}

async fn cmd_scan() -> Result<()> {
    println!("Scanning for devices ({SCAN_TIMEOUT:?})...");
    let results = ble::scan(SCAN_TIMEOUT, protocol::BLE_DEVICE_NAME).await?;
    if results.is_empty() {
        println!("No devices found.");
        return Ok(());
    }
    for r in results {
        println!("  {}  {}  (RSSI: {})", r.address, r.name, r.rssi);
    }
    Ok(())
}

async fn cmd_status(cli: &Cli) -> Result<()> {
    let mut client = connect(cli).await?;
    let result = client.get_status(STATUS_TIMEOUT).await;
    let _ = client.disconnect().await;
    print_status(&result?);
    Ok(())
}

async fn cmd_temp(cli: &Cli, left: Option<f64>, right: Option<f64>, verify: bool) -> Result<()> {
    if left.is_none() && right.is_none() {
        bail!("specify --left and/or --right temperature");
    }
    if let Some(t) = left {
        if !(28.0..=48.0).contains(&t) {
            bail!("left temperature must be between 28.0 and 48.0 (got {t:.1})");
        }
    }
    if let Some(t) = right {
        if !(28.0..=48.0).contains(&t) {
            bail!("right temperature must be between 28.0 and 48.0 (got {t:.1})");
        }
    }

    let mut client = connect(cli).await?;
    let outcome = temp_inner(&client, left, right, verify).await;
    let _ = client.disconnect().await;
    outcome
}

/// 온도 설정 본체. 한쪽만 지정 시 반대쪽 목표 온도를 유지한다.
async fn temp_inner(
    client: &ble::Client,
    left: Option<f64>,
    right: Option<f64>,
    verify: bool,
) -> Result<()> {
    let (side, left_temp, right_temp) = match (left, right) {
        (Some(l), Some(r)) => (protocol::SIDE_BOTH, l, r),
        (Some(l), None) => {
            let st = client
                .get_status(STATUS_TIMEOUT)
                .await
                .context("failed to get current status")?;
            // 전원 OFF 상태(목표 0)면 동일 온도 사용
            let r = if st.right_target == 0.0 {
                l
            } else {
                st.right_target
            };
            (protocol::SIDE_LEFT, l, r)
        }
        (None, Some(r)) => {
            let st = client
                .get_status(STATUS_TIMEOUT)
                .await
                .context("failed to get current status")?;
            let l = if st.left_target == 0.0 {
                r
            } else {
                st.left_target
            };
            (protocol::SIDE_RIGHT, l, r)
        }
        (None, None) => unreachable!("validated above"),
    };

    client.set_temp(side, left_temp, right_temp).await?;
    tokio::time::sleep(POST_COMMAND_DELAY).await;
    println!("Temperature set: left={left_temp:.1}°C right={right_temp:.1}°C");

    if verify {
        verify_status(client).await?;
    }
    Ok(())
}

async fn cmd_on(cli: &Cli, verify: bool) -> Result<()> {
    let mut client = connect(cli).await?;
    let outcome = async {
        client.power_on().await?;
        tokio::time::sleep(POST_COMMAND_DELAY).await;
        println!("Power ON sent.");
        if verify {
            verify_status(&client).await?;
        }
        Ok(())
    }
    .await;
    let _ = client.disconnect().await;
    outcome
}

async fn cmd_off(cli: &Cli, verify: bool) -> Result<()> {
    let mut client = connect(cli).await?;
    let outcome = async {
        client.power_off().await?;
        tokio::time::sleep(POST_COMMAND_DELAY).await;
        println!("Power OFF sent.");
        if verify {
            verify_status(&client).await?;
        }
        Ok(())
    }
    .await;
    let _ = client.disconnect().await;
    outcome
}

async fn cmd_setup(cli: &Cli, reset: bool) -> Result<()> {
    if reset {
        config::delete().context("failed to delete config")?;
        match config::config_path() {
            Ok(path) => println!("Config deleted: {}", path.display()),
            Err(_) => println!("Config deleted."),
        }
        return Ok(());
    }

    // 기존 설정 확인
    let existing = config::load().context("failed to load existing config")?;
    if let Some(existing) = &existing {
        println!(
            "Existing config found (address: {}, GID: {})",
            existing.address,
            mask_device_gid(&existing.device_gid)
        );
        print!("Overwrite? [y/N]: ");
        flush_stdout();
        match read_line()? {
            None => {
                println!("Setup cancelled.");
                return Ok(());
            }
            Some(answer) => {
                let answer = answer.to_lowercase();
                if answer != "y" && answer != "yes" {
                    println!("Setup cancelled.");
                    return Ok(());
                }
            }
        }
    }

    // [1/4] 스캔
    println!("\n[1/4] Scanning for devices...");
    let results = ble::scan(SCAN_TIMEOUT, protocol::BLE_DEVICE_NAME)
        .await
        .context("scan failed (check BLE permissions)")?;
    let unique = dedupe_sorted(results);
    if unique.is_empty() {
        bail!(
            "no {} device found. Make sure the mat is powered on and in range",
            protocol::BLE_DEVICE_NAME
        );
    }

    // 기기 선택
    let selected_addr = if unique.len() == 1 {
        println!(
            "Found device: {} (RSSI: {})",
            unique[0].address, unique[0].rssi
        );
        unique[0].address.clone()
    } else {
        println!("Found {} devices:", unique.len());
        for (i, r) in unique.iter().enumerate() {
            println!("  [{}] {} (RSSI: {})", i + 1, r.address, r.rssi);
        }
        print!("Select device number: ");
        flush_stdout();
        let input = read_line()?.context("no input received")?;
        let idx: usize = input
            .parse()
            .ok()
            .filter(|&n| n >= 1 && n <= unique.len())
            .with_context(|| format!("invalid selection: {input}"))?;
        unique[idx - 1].address.clone()
    };

    // [2/4] DeviceGid 취득 방식 선택
    println!("\n[2/4] DeviceGid acquisition");
    println!("  [1] Enter existing GID (from official app)");
    println!("  [2] New pairing (mat must be in pairing mode)");
    print!("Select [1/2]: ");
    flush_stdout();
    let choice = read_line()?.context("no input received")?;

    let gid = match choice.as_str() {
        "1" => {
            print!("Enter DeviceGid (12-char hex, e.g. 13CE3CC53E5A): ");
            flush_stdout();
            let gid_hex = read_line()?.context("no input received")?;
            protocol::DeviceGid::from_hex(&gid_hex).context("invalid GID")?
        }
        "2" => {
            println!("Starting pairing... (make sure mat is in pairing mode)");
            let mut pair_client = ble::Client::new(
                selected_addr.clone(),
                protocol::DeviceGid::new([0u8; 6]),
                cli.debug,
            );
            let result = pair_client.pair().await;
            let _ = pair_client.disconnect().await;
            let gid = result.context("pairing failed")?;
            println!("DeviceGid acquired: {}", mask_device_gid(&gid.to_string()));
            gid
        }
        other => bail!("invalid choice: {other}"),
    };

    // [3/4] 연결 검증
    println!("\n[3/4] Verifying connection...");
    let mut client = ble::Client::new(selected_addr.clone(), gid, cli.debug);
    let verify_result = async {
        client.connect().await.context("verification failed")?;
        client
            .get_status(STATUS_TIMEOUT)
            .await
            .context("status check failed")
    }
    .await;
    let _ = client.disconnect().await;
    let st = verify_result?;

    if st.powered_off {
        println!("Connection verified! (mat is powered off)");
    } else {
        println!(
            "Connection verified! Mode: {}, Left: {:.1}°C, Right: {:.1}°C",
            st.mode_name, st.left_current, st.right_current
        );
    }

    // [4/4] 설정 저장
    println!("\n[4/4] Saving config...");
    let cfg = DeviceConfig {
        address: selected_addr,
        device_gid: gid.to_string(),
        cached_at: 0,
    };
    config::save(&cfg).context("save config failed")?;
    match config::config_path() {
        Ok(path) => println!("Config saved to: {}", path.display()),
        Err(_) => println!("Config saved."),
    }
    println!("\nSetup complete! You can now use commands without --address and --device-gid.");
    Ok(())
}

/// 플래그/환경변수/설정 파일/자동 스캔을 기반으로 BLE 기기에 연결한다.
///
/// 주소: --address → HOTWATERMAT_ADDRESS → 설정 파일 → 자동 스캔.
/// GID: --device-gid → HOTWATERMAT_DEVICE_GID → 설정 파일 → 오류.
/// 연결 성공 시 설정 파일이 없었다면 자동 저장한다.
async fn connect(cli: &Cli) -> Result<ble::Client> {
    // clap이 플래그와 환경변수를 이미 병합해 둔다.
    let mut address = cli.address.clone().unwrap_or_default();

    // 설정 파일은 항상 로드 시도. 파일이 없을 때만(can_autosave) 이후 자동 저장한다.
    let (cached, can_autosave) = match config::load() {
        Ok(Some(c)) => (Some(c), false),
        Ok(None) => (None, true),
        Err(e) => {
            eprintln!("Warning: config load failed: {e}");
            (None, false)
        }
    };

    if address.is_empty() {
        if let Some(c) = &cached {
            if !c.address.is_empty() {
                address = c.address.clone();
                eprintln!("Using cached address: {address}");
            }
        }
    }

    if address.is_empty() {
        println!("No address specified, scanning for devices...");
        let results = ble::scan(SCAN_TIMEOUT, protocol::BLE_DEVICE_NAME)
            .await
            .context("auto-scan failed")?;
        let unique = dedupe_sorted(results);
        match unique.len() {
            0 => bail!(
                "no {} device found. Make sure the mat is powered on and in range",
                protocol::BLE_DEVICE_NAME
            ),
            1 => {
                address = unique[0].address.clone();
                println!("Found device: {} (RSSI: {})", address, unique[0].rssi);
            }
            _ => {
                println!("Multiple devices found:");
                for (i, r) in unique.iter().enumerate() {
                    println!("  [{}] {} (RSSI: {})", i + 1, r.address, r.rssi);
                }
                println!("Please specify --address to select a device.");
                bail!("multiple devices found, specify --address");
            }
        }
    }

    // GID 결정: 플래그/환경변수 → 설정 파일 → 오류
    let gid = if let Some(g) = &cli.device_gid {
        protocol::DeviceGid::from_hex(g)
            .context("invalid device-gid (flag or HOTWATERMAT_DEVICE_GID)")?
    } else if let Some(c) = cached.as_ref().filter(|c| !c.device_gid.is_empty()) {
        let gid =
            protocol::DeviceGid::from_hex(&c.device_gid).context("invalid cached device_gid")?;
        if cli.debug {
            eprintln!("Using cached device GID: {}", c.device_gid);
        }
        gid
    } else {
        bail!("no device GID specified. Run 'hotwatermat-ble setup' to pair, or set HOTWATERMAT_DEVICE_GID / --device-gid");
    };

    let mut client = ble::Client::new(address.clone(), gid, cli.debug);
    client.connect().await?;

    // 설정 파일이 없었으면 자동 저장
    if can_autosave {
        let cfg = DeviceConfig {
            address,
            device_gid: gid.to_string(),
            cached_at: 0,
        };
        if let Err(e) = config::save(&cfg) {
            eprintln!("Warning: failed to auto-save config: {e}");
        }
    }

    Ok(client)
}

/// 매트 상태를 출력한다.
fn print_status(st: &protocol::Status) {
    if st.powered_off {
        println!("Power: OFF");
        return;
    }
    println!("Mode:    {}", st.mode_name);
    println!("Side:    {}", protocol::side_name(st.side));
    println!("Water:   {}", protocol::water_level_name(st.water_level));
    println!(
        "Left:    {:.1}°C → {:.1}°C",
        st.left_current, st.left_target
    );
    println!(
        "Right:   {:.1}°C → {:.1}°C",
        st.right_current, st.right_target
    );
}

/// 명령 실행 후 1초 대기한 뒤 상태를 다시 조회하여 출력한다(--verify).
async fn verify_status(client: &ble::Client) -> Result<()> {
    tokio::time::sleep(Duration::from_secs(1)).await;
    client.clear_status(); // 캐시 제거 → 새 STATUS 대기
    let st = client
        .get_status(STATUS_TIMEOUT)
        .await
        .context("verify failed")?;
    println!("\n[Verify]");
    print_status(&st);
    Ok(())
}

/// 스캔 결과에서 주소별 최대 RSSI만 남기고 RSSI 내림차순(동률이면 주소 오름차순)으로 정렬한다.
fn dedupe_sorted(results: Vec<ScanResult>) -> Vec<ScanResult> {
    let mut best: HashMap<String, ScanResult> = HashMap::new();
    for r in results {
        match best.get(&r.address) {
            Some(prev) if prev.rssi >= r.rssi => {}
            _ => {
                best.insert(r.address.clone(), r);
            }
        }
    }
    let mut unique: Vec<ScanResult> = best.into_values().collect();
    unique.sort_by(|a, b| b.rssi.cmp(&a.rssi).then_with(|| a.address.cmp(&b.address)));
    unique
}

/// DeviceGid를 마스킹한다(앞 2자리 + **** + 뒤 2자리).
fn mask_device_gid(gid: &str) -> String {
    if gid.len() <= 4 {
        return "****".to_string();
    }
    format!("{}****{}", &gid[..2], &gid[gid.len() - 2..])
}

/// stdin에서 한 줄을 읽어 trim한다. EOF면 `None`.
fn read_line() -> Result<Option<String>> {
    use std::io::BufRead;
    let mut s = String::new();
    let n = std::io::stdin()
        .lock()
        .read_line(&mut s)
        .context("read input")?;
    if n == 0 {
        return Ok(None);
    }
    Ok(Some(s.trim().to_string()))
}

fn flush_stdout() {
    use std::io::Write;
    let _ = std::io::stdout().flush();
}
