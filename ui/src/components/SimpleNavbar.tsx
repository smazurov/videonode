import { Link } from "react-router-dom";
import React from "react";
import Container from "./Container";

interface Props { 
  logoHref?: string; 
  actionElement?: React.ReactNode;
  logoText?: string;
}

export default function SimpleNavbar({ logoHref, actionElement, logoText = "VideoNode" }: Readonly<Props>) {
  return (
    <div>
      <Container>
        <div className="pb-4 my-4 border-b border-slate-800/20 isolate dark:border-slate-300/20">
          <div className="flex items-center justify-between">
            <Link to={logoHref ?? "/"} className="h-[26px]">
              <div className="text-xl font-bold text-black dark:text-white">
                {logoText}
              </div>
            </Link>
            <div>{actionElement}</div>
          </div>
        </div>
      </Container>
    </div>
  );
}