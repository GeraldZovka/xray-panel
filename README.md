# Xray Panel — VLESS + REALITY

Panel web ringan berbasis Go untuk mengelola satu instance Xray dengan banyak akun VLESS. Dibuat untuk VPS Ubuntu ARM64/amd64 dengan RAM terbatas dan NAT yang memetakan port publik ke port internal berbeda.

> **Penting:** isi `GeraldZovka/xray-panel` dengan pemilik dan nama repo GitHub Anda. Installer meng-clone repo yang sama ke `/opt/xray-panel`; URL `raw.githubusercontent.com` saja tidak cukup untuk mengetahui URL clone-nya.

## Isi repo

```text
.
├── install.sh                 # install, update, uninstall
├── panel.go                   # server HTTP, autentikasi, API, state
├── config.go                  # validasi, generator config/link VLESS, key
├── config_test.go             # unit test generator dan validasi
├── panel_test.go              # test izin file state
├── web.go                     # embed UI ke binary
├── web/index.html             # UI responsif, tanpa CDN
├── cmd/helper/main.go         # helper root dengan aksi yang dibatasi
├── cmd/helper/main_test.go    # test config mode 0600
├── systemd/xray-panel.service # service panel non-root
├── sudoers/xray-panel         # izin helper root tanpa argumen
├── go.mod, go.sum
├── .gitignore                 # mengabaikan binary dan data rahasia
└── vendor/                    # dependency Go vendored untuk build offline dari module proxy
```

## Instalasi satu baris

Jalankan sebagai `root` di Ubuntu 22.04 atau lebih baru. Bentuk yang diminta:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/GeraldZovka/xray-panel/main/install.sh)
```

Ganti `GeraldZovka/xray-panel` pada URL raw dengan repo Anda; installer lalu meminta URL clone HTTPS repo untuk menyimpannya ke `/opt/xray-panel`. Agar tak perlu prompt itu, jalankan dengan `XRAY_PANEL_REPO=https://github.com/GeraldZovka/xray-panel.git` sebelum `bash`.

Installer meminta alamat publik, port publik/internal panel, port publik/internal Xray, password admin, dan pilihan TLS. Default yang dipilih:

- Host publik: `vectis-nat-1-20151-ixt.vps-eu1.sumopod.my.id`
- Panel: publik `8080` → internal `8080`
- Xray: publik `20151` → internal `443`
- REALITY SNI/dest awal: `www.cloudflare.com` / `www.cloudflare.com:443`
- Koneksi awal: VLESS + TCP + REALITY + Vision

Port panel `8080`, port internal Xray `443`, dan target REALITY adalah **asumsi awal yang dapat diubah**. Installer mengecek agar port publik panel dan Xray tidak sama, port internal keduanya berbeda, lalu mengecek port internal yang benar-benar akan di-bind menggunakan daftar listener TCP lokal dan berhenti jika port terpakai. Port publik NAT bukan socket lokal, sehingga tidak bisa diperiksa dari VPS; pastikan port itu dialokasikan/dipetakan di panel provider.

Jika password admin dikosongkan, installer membuat password acak dan menampilkannya **sekali setelah state tersimpan**. Simpan di password manager. Rerun installer tidak mereset password, key, user, atau pengaturan yang tersimpan.

Installer memasang dependency OS dari repositori Ubuntu, meng-clone repo, membangun Go binary dengan satu worker, memasang Xray dari [installer resmi XTLS](https://github.com/XTLS/Xray-install), menyimpan backup config lama, dan memasang unit systemd. Source module Go sudah berada di `vendor/`; build tidak perlu meminta dependency ke Go module proxy.

## NAT, port, dan TCP saja

NAT provider harus mengarahkan **panel publik → panel internal** dan **Xray publik → Xray internal**, keduanya sebagai TCP. Contoh default: `8080 → 8080` untuk panel dan `20151 → 443` untuk Xray. Jika provider hanya memberi mapping Xray `20151 → 443`, panel juga tetap memerlukan mapping TCP publik tersendiri; alamat domain saja tidak membuka port.

UFW, bila aktif, diizinkan untuk port internal TCP. UDP tidak dipakai oleh listener proxy; aturan routing Xray mengirim trafik UDP ke outbound `blackhole`. Xray tetap meneruskan trafik TCP pengguna ke tujuan yang diminta sebagai fungsi proxy biasa. Pastikan firewall/provider juga tidak memaksa UDP.

Port publik Xray yang muncul di `vless://` adalah port provider, sedangkan Xray bind ke port internal. Mengubah port Xray dari panel akan memvalidasi config dan me-restart service; NAT provider dan aturan UFW/provider tidak berubah otomatis. Buka port TCP internal baru secara manual bila UFW aktif.

## Update dan uninstall

Perbarui panel dan Xray, pertahankan state/password/key/user, lalu validasi config sebelum restart:

```bash
XRAY_PANEL_REPO=https://github.com/GeraldZovka/xray-panel.git bash <(curl -fsSL https://raw.githubusercontent.com/GeraldZovka/xray-panel/main/install.sh) update
```

Tombol **Perbarui Xray** di panel hanya menjalankan updater Xray resmi. Pembaruan dapat memutus sesi proxy aktif sesaat. Jika SSH Anda sendiri melewati Xray, update/restart dapat memutus koneksi SSH.

Uninstall panel (meminta konfirmasi dengan mengetik `uninstall`):

```bash
XRAY_PANEL_REPO=https://github.com/GeraldZovka/xray-panel.git bash <(curl -fsSL https://raw.githubusercontent.com/GeraldZovka/xray-panel/main/install.sh) uninstall
```

Untuk menghindari penghapusan layanan VPS lain, `uninstall` **hanya** menghapus panel, helper dan unit panel. Xray tetap berjalan dengan config aktif dan drop-in credential-nya; data disalin ke `/var/backups/xray-panel-uninstalled-*`, file data aslinya, environment `/etc/xray-panel.env`, sertifikat TLS, akun sistem `xray-panel`, aturan UFW, clone `/opt/xray-panel`, dan instalasi Xray tidak dihapus. Environment/state dipertahankan agar pemasangan ulang tetap idempoten; pembersihan tersebut sengaja manual.

## Panel dan fitur

- Buka URL panel yang dicetak installer, misalnya `http://HOST:8080`.
- Login memakai password admin. Password disimpan sebagai bcrypt; sesi cookie `HttpOnly`, `SameSite=Strict`, token CSRF, pemeriksaan Origin, dan rate limit 5 gagal/15 menit per IP.
- Dashboard menunjukkan status/versi/uptime Xray, estimasi CPU host (sampel singkat), load average host, perkiraan RSS proses Xray, kontrol service, dan log terakhir.
- Tambah, nonaktifkan, aktifkan, atau hapus pengguna. Setiap user mendapat UUID. Link VLESS dapat disalin dan QR dibuat lokal tanpa CDN.
- Pilih **satu mode aktif** untuk instance ini: `tcp` (Vision) atau `xhttp`. Beralih mode mengubah link semua user karena hanya satu inbound/port internal yang dikelola.
- Pengaturan publik/SNI/dest/port diperiksa server-side. Config baru ditulis ke file sementara, diuji memakai `xray run -test -config`, lalu diterapkan dan service direstart. Jika uji atau restart gagal, helper root memulihkan config sebelumnya; UI menampilkan error tanpa private key.
- Backup adalah file JSON yang mengandung key REALITY privat untuk bisa memulihkan koneksi. File tidak dirender ke halaman, tetapi tetap rahasia: simpan terenkripsi dan jangan unggah ke repo.
- Dukungan HTTPS: installer dapat membuat sertifikat self-signed, atau memakai cert/key yang Anda sediakan. Sertifikat self-signed tetap menghasilkan peringatan browser. Untuk sertifikat terpercaya, gunakan domain dan sertifikat CA; reverse proxy TLS juga bisa dipakai. Tanpa TLS, panel menampilkan peringatan HTTP.

### Pengaturan TLS setelah install

Aplikasi menerima `PANEL_TLS_CERT` dan `PANEL_TLS_KEY` melalui `/etc/xray-panel.env`. Jika disetel, file cert/key harus dapat dibaca user `xray-panel` (installer mengaturnya saat setup), lalu:

```bash
sudo systemctl restart xray-panel
```

Untuk reverse proxy HTTPS, bind panel ke loopback dengan mengubah `PANEL_ADDR=127.0.0.1:8080` pada `/etc/xray-panel.env` dan restart service. Bila reverse proxy berjalan lokal, panel mengenali `X-Forwarded-Proto: https` hanya dari koneksi loopback.

## Keamanan dan pilihan desain

- Proses panel berjalan sebagai user Linux non-root `xray-panel`. Satu helper root yang root-owned hanya menerima aksi JSON tetap: apply config, status/log, start/stop/restart Xray, dan update resmi. Sudoers tidak memberi shell atau perintah bebas; helper juga menolak semua argumen command line.
- Admin panel berwenang mengelola proxy dan meminta update Xray, jadi gunakan password unik yang panjang dan batasi akses jaringan ke port panel. Jangan expose panel HTTP ke Internet tanpa HTTPS.
- State panel (`/var/lib/xray-panel/state.json`), config Xray aktif, dan backup config diberi mode `0600`; direktori data panel `0700`. Config aktif ber-owner `root:root`; drop-in systemd memakai `LoadCredential` untuk menyerahkan salinan read-only hanya kepada service Xray non-root saat start, sehingga user `xray` tidak dapat membaca file sumber secara langsung. Ini memerlukan systemd 249 atau lebih baru (Ubuntu 22.04+).
- Private key REALITY disimpan di state/config, tidak dikembalikan pada API biasa, link, QR, atau log UI. Backup yang diunduh memang berisi private key dan harus diperlakukan sebagai rahasia.
- Tidak ada analytics, telemetry, CDN, atau API eksternal di runtime panel. Akses jaringan installer mencakup repo GitHub, GitHub untuk Xray, dan mirror Ubuntu untuk paket OS; permintaan update Xray dari helper mengunduh installer resmi GitHub. Xray sendiri tentu membuat koneksi keluar untuk meneruskan trafik TCP klien proxy.
- Kuota/kedaluwarsa dan statistik trafik per-user belum disertakan. Xray Stats API tidak diaktifkan agar jejak pemakaian dan kompleksitas tetap minimal.

## Checklist pengujian manual

Lakukan lebih dulu pada VPS uji atau dengan backup yang dapat dipulihkan.

1. **Konflik port:** sebelum instal, pilih port internal yang sudah listen (misalnya `22` jika SSH menggunakannya), masukkan sebagai port internal panel, lalu jalankan installer. Pastikan installer menampilkan proses/port yang bentrok dan berhenti sebelum mengganti config.
2. **Instalasi baru:** pastikan installer selesai, `systemctl status xray xray-panel`, dan `ss -ltnp` menampilkan hanya port internal yang diharapkan. Pastikan tidak ada listener UDP dari panel ini.
3. **NAT dan firewall:** konfirmasi mapping TCP publik→internal di provider, akses URL panel dari luar VPS, dan tes koneksi TCP ke port publik Xray. Uji juga dari jaringan berbeda jika memungkinkan.
4. **Login:** password benar masuk; salah 5 kali dibatasi; cookie panel memiliki `HttpOnly`/`SameSite=Strict`. Jika HTTP, peringatan keamanan terlihat.
5. **Pengguna dan QR:** tambah user, salin link, pindai QR di klien VLESS yang mendukung REALITY + Vision. Tes koneksi TCP dan akses situs; pastikan key privat tidak ada di link, QR, endpoint state, atau log panel.
6. **Nonaktif/hapus user:** nonaktifkan user dan pastikan koneksi barunya gagal, aktifkan kembali lalu uji; hapus user dan pastikan hilang dari daftar/config.
7. **Perubahan gagal:** masukkan dest atau XHTTP path yang tidak valid, simpan, pastikan UI menunjukkan error dan config sebelumnya tetap bekerja. Untuk uji rollback restart, lakukan hanya di staging dengan port internal yang sengaja bentrok.
8. **Mode XHTTP:** ubah ke XHTTP, buat/scan ulang link dengan klien yang mendukung VLESS XHTTP REALITY, lalu uji. Kembali ke TCP Vision bila klien tidak mendukungnya.
9. **Backup/restore:** unduh backup, simpan secara aman, ubah user/settings, restore backup dan pastikan config aktif dipulihkan. Pastikan restore tidak mengganti password admin yang sedang dipakai.
10. **Password/TLS:** ubah password admin, pastikan sesi lama harus login ulang; bila TLS diaktifkan, buka `https://HOST:PORT` dan verifikasi bahwa browser menunjukkan TLS (sertifikat self-signed akan tetap diberi warning).
11. **Update/uninstall:** pada staging, uji update dan pastikan pengguna tetap ada. Uji uninstall dan pastikan panel berhenti tetapi Xray/config aktif tetap tersedia.

## Troubleshooting

- **Panel tidak dapat dibuka:** periksa `systemctl status xray-panel`, `journalctl -u xray-panel -n 100 --no-pager`, `ss -ltnp`, UFW, lalu pastikan mapping publik panel mengarah ke port internal yang benar.
- **Xray gagal restart / port conflict:** periksa `systemctl status xray`, `journalctl -u xray -n 100 --no-pager`, dan `ss -ltnp`. Perubahan melalui panel mengembalikan config sebelumnya saat restart gagal. Periksa pesan error yang ditampilkan UI.
- **Tidak ada koneksi klien:** pastikan alamat publik dan port publik di link benar, mapping provider TCP aktif, internal port Xray dan UFW sesuai, UUID/REALITY public key/short ID benar, dan SNI/dest mendukung TLS 1.3. Pastikan klien memakai transport yang sedang dipilih.
- **Update gagal:** pastikan VPS dapat mengakses GitHub dan mirror Ubuntu, ruang disk cukup, lalu jalankan `install.sh update` lagi setelah memeriksa status Xray. Update resmi menggunakan `--manually-restart-later`; script kemudian menjalankan tes config dan restart.
- **Lupa password:** tidak ada mekanisme reset dari UI. Setelah login shell root ke VPS, gunakan prosedur reset yang Anda audit sendiri atau backup/restore; jangan mengedit state sembarangan karena memuat key rahasia.
- **Panel tetap HTTP:** file TLS harus pasangan cert/key yang benar, terbaca oleh user `xray-panel`, dan tercantum di `/etc/xray-panel.env`; lihat log service setelah restart.

## Verifikasi yang dilakukan pada paket sumber

Di lingkungan pengembangan: `go test -mod=vendor ./...`, `go vet`, build binary panel/helper, `bash -n install.sh`, syntax `systemd-analyze verify` untuk `LoadCredential`, dan pemeriksaan `xray run -test -config` untuk mode TCP, mode XHTTP, serta config awal tanpa user menggunakan binary resmi Xray `26.10.31` dijalankan; semua config menjawab `Configuration OK`. Smoke test HTTP lokal dengan helper tiruan juga memeriksa login, penolakan CSRF, tambah user, redaksi private key dari API, QR PNG, dan backup. **Belum** diuji sebagai service systemd di VPS, pada NAT provider, dengan UFW aktif, atau klien VLESS eksternal. Checklist di atas tetap perlu dijalankan setelah instalasi VPS.
