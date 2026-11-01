import flower from "../assets/logo.svg";

/** The flower mark next to the lowercase wordmark, as in the README banner. */
export function Logo({ className = "text-lg", markClassName = "size-6" }: { className?: string; markClassName?: string }) {
    return (
        <span className={`bloom-wordmark inline-flex items-center gap-2 ${className}`}>
            <img src={flower} alt="" aria-hidden="true" className={markClassName} />
            <span>bloom</span>
        </span>
    );
}
