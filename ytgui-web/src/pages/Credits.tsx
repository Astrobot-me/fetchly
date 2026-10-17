import { ArrowUpRight, Bolt, Code, Star } from "../components/Icons";
import { AUTHOR, REPO } from "../credits";
import { useStore } from "../state/store";

const ext = { target: "_blank", rel: "noopener noreferrer" } as const;

export function Credits() {
  const { tools } = useStore();
  return (
    <div className="page page--credits">
      <header className="credits-hero">
        <span className="credits-hero__mark" aria-hidden="true">
          <Bolt size={34} />
        </span>
        <p className="eyebrow">Credits</p>
        <h1 className="credits-hero__title">
          Made by <span className="hero__accent">{AUTHOR.name}</span>
        </h1>
        <p className="credits-hero__links">
          <a className="textlink" href={AUTHOR.github} {...ext}>
            @{AUTHOR.handle}
            <ArrowUpRight size={14} />
          </a>
          <a className="textlink" href={REPO} {...ext}>
            Source code on GitHub
            <ArrowUpRight size={14} />
          </a>
        </p>
      </header>

      <section className="card starcard" aria-labelledby="star-title">
        <span className="starcard__icon" aria-hidden="true">
          <Star size={28} />
        </span>
        <div className="starcard__body">
          <h2 id="star-title" className="starcard__title">
            If Fetchly is useful to you, please ⭐ star it on GitHub
          </h2>
          <p className="muted">It's free and open source. A star helps other people find it.</p>
        </div>
        <a className="btn btn--primary btn--xl" href={REPO} {...ext}>
          <Star size={18} />
          Star on GitHub
        </a>
      </section>

      <section className="card section" aria-labelledby="about-title">
        <div className="section__head">
          <h2 id="about-title" className="section-title">
            About this app
          </h2>
          <span className="version">
            <Code size={14} />
            Version {tools?.appVersion ?? "—"}
          </span>
        </div>
        <p>
          Fetchly uses these free programs behind the scenes. They are downloaded from their official sources.
        </p>
        <ul className="notices">
          <li>
            <span className="notices__name">yt-dlp</span>
            <span className="notices__lic">Unlicense</span>
          </li>
          <li>
            <span className="notices__name">FFmpeg</span>
            <span className="notices__lic">GPL builds by BtbN on Windows and Martin Riedl on macOS</span>
          </li>
          <li>
            <span className="notices__name">Deno</span>
            <span className="notices__lic">MIT</span>
          </li>
        </ul>
        <p className="notice-strong">Only download videos you have the right to download.</p>
      </section>
    </div>
  );
}
